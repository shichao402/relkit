// Package agentadmin owns the one place an agent server records who may sign
// in to its management surface and which products a publisher token may touch.
//
// It replaces the config-file spelling of products and upload tokens. A config
// file now carries only the listen address, state dir, custody manifest and
// deployment-level knobs; registering a product or issuing a token is an agent
// operation, not an SSH edit (requirements 11.3, 13.6). Tokens are stored as
// sha256 hashes, so a stolen state file yields no usable credential, and the
// plaintext exists only in the response that issued it.
package agentadmin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/shichao402/relkit/internal/model"
)

// Schema names this document. The epoch upgrade keeps no reader for the older
// config-file spelling on purpose: a server that still has `products` or
// `uploadTokens` in its JSON refuses to start rather than guessing (demand 1).
const Schema = "relkit.agent-admin/1"

// Role decides what a signed-in user may change. There are exactly two, because
// the panel has exactly three subjects: users, roles, products (requirement 13.3).
type Role string

const (
	// RoleAdmin manages users and every product.
	RoleAdmin Role = "admin"
	// RoleOwner manages the tokens of the products assigned to it, nothing else.
	RoleOwner Role = "owner"
)

// MinPasswordLength mirrors the console panel rule so one operator password
// works in both surfaces.
const MinPasswordLength = 8

// DefaultSessionTTL is how long a management session stays valid.
const DefaultSessionTTL = 12 * time.Hour

// User is one management account. PasswordHash is bcrypt; there is no plaintext
// anywhere on disk.
type User struct {
	Username     string   `json:"username"`
	PasswordHash string   `json:"passwordHash"`
	Role         Role     `json:"role"`
	Products     []string `json:"products,omitempty"`
	CreatedAt    string   `json:"createdAt"`
}

// Product is one host product this server publishes. Root is the product tree
// on this host; Profile is the machine publish profile the agent reads when it
// needs backends. Neither is a secret.
type Product struct {
	ID        string `json:"id"`
	Root      string `json:"root"`
	Profile   string `json:"profile,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// Token is one publisher credential. Hash is the lowercase hex sha256 of the
// plaintext; the plaintext is returned exactly once, by Issue or Rotate.
type Token struct {
	ID        string `json:"id"`
	Product   string `json:"product"`
	Label     string `json:"label,omitempty"`
	Hash      string `json:"hash"`
	CreatedAt string `json:"createdAt"`
	RotatedAt string `json:"rotatedAt,omitempty"`
	RevokedAt string `json:"revokedAt,omitempty"`
}

// Revoked reports whether this token is dead. A revoked token stays in the
// document so the panel can show what was issued and when it was withdrawn.
func (t Token) Revoked() bool { return t.RevokedAt != "" }

// Document is the whole state file. Sessions are deliberately absent: they are
// process state, so a restart signs every operator out.
type Document struct {
	Schema        string    `json:"schema"`
	BootstrapHash string    `json:"bootstrapHash,omitempty"`
	Users         []User    `json:"users,omitempty"`
	Products      []Product `json:"products,omitempty"`
	Tokens        []Token   `json:"tokens,omitempty"`
}

type session struct {
	username  string
	expiresAt time.Time
}

// Store is the in-memory view of one admin document plus its sessions. Every
// mutation persists before it returns, so a crash never leaves the panel and
// the file disagreeing.
type Store struct {
	mu       sync.Mutex
	path     string
	doc      Document
	sessions map[string]session
	now      func() time.Time
}

// Open reads the admin document at path. A missing file is an empty server: no
// users (so the panel is locked), no products, no tokens. Nothing is written
// until something changes, so a read-only mirror never grows a state file.
func Open(path string) (*Store, error) {
	store := &Store{
		path:     path,
		sessions: map[string]session{},
		now:      time.Now,
		doc:      Document{Schema: Schema},
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		return store, nil
	}
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Schema != Schema {
		return nil, fmt.Errorf("%s: schema is %q, expected %q", path, doc.Schema, Schema)
	}
	if err := validateDocument(doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	store.doc = doc
	return store, nil
}

// Path is the state file this store persists to.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func validateDocument(doc Document) error {
	seenUsers := map[string]bool{}
	for index, user := range doc.Users {
		name, err := NormalizeUsername(user.Username)
		if err != nil {
			return fmt.Errorf("users[%d]: %w", index, err)
		}
		if name != user.Username {
			return fmt.Errorf("users[%d]: username %q must be stored lowercase", index, user.Username)
		}
		if seenUsers[name] {
			return fmt.Errorf("users[%d]: duplicate username %q", index, name)
		}
		seenUsers[name] = true
		if user.PasswordHash == "" {
			return fmt.Errorf("users[%d]: passwordHash is required", index)
		}
		if user.Role != RoleAdmin && user.Role != RoleOwner {
			return fmt.Errorf("users[%d]: role %q is not %q or %q", index, user.Role, RoleAdmin, RoleOwner)
		}
		for _, id := range user.Products {
			if err := model.CheckIdentifier(id, "product"); err != nil {
				return fmt.Errorf("users[%d]: %w", index, err)
			}
		}
	}
	seenProducts := map[string]bool{}
	for index, product := range doc.Products {
		if err := model.CheckIdentifier(product.ID, "product"); err != nil {
			return fmt.Errorf("products[%d]: %w", index, err)
		}
		if seenProducts[product.ID] {
			return fmt.Errorf("products[%d]: duplicate product %q", index, product.ID)
		}
		seenProducts[product.ID] = true
		if strings.TrimSpace(product.Root) == "" {
			return fmt.Errorf("products[%d]: root is required", index)
		}
	}
	seenTokens := map[string]bool{}
	for index, token := range doc.Tokens {
		if token.ID == "" {
			return fmt.Errorf("tokens[%d]: id is required", index)
		}
		if seenTokens[token.ID] {
			return fmt.Errorf("tokens[%d]: duplicate id %q", index, token.ID)
		}
		seenTokens[token.ID] = true
		if !seenProducts[token.Product] {
			return fmt.Errorf("tokens[%d]: product %q is not registered", index, token.Product)
		}
		if len(token.Hash) != sha256.Size*2 {
			return fmt.Errorf("tokens[%d]: hash is not a sha256 hex digest", index)
		}
	}
	return nil
}

// persist writes the current document with 0600 and a rename, so a panel read
// never sees half a file.
func (s *Store) persistLocked() error {
	if s.path == "" {
		return fmt.Errorf("admin state path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp~"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		// Windows cannot rename over an existing file.
		if removeErr := os.Remove(s.path); removeErr == nil || os.IsNotExist(removeErr) {
			if renameErr := os.Rename(tmp, s.path); renameErr == nil {
				return nil
			}
		}
		os.Remove(tmp)
		return err
	}
	return nil
}

// BootstrapToken mints the one-shot credential that creates the first admin. It
// is refused once an account exists: there is nothing left for it to authorise.
// It is also refused while a previous credential is still live, because minting
// twice would silently invalidate the value an operator already copied; use
// ResetBootstrap to replace it deliberately.
func (s *Store) BootstrapToken() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.doc.Users) > 0 {
		return "", fmt.Errorf("an operator account already exists; the bootstrap is retired")
	}
	if len(s.doc.BootstrapHash) == sha256.Size*2 {
		return "", fmt.Errorf("a bootstrap credential is already live; use it, or run with -reset to replace it")
	}
	return s.mintBootstrapLocked()
}

// ResetBootstrap replaces a live bootstrap credential. It exists so an operator
// who lost the value has an explicit, documented way back in instead of
// discovering that a second `init -bootstrap` quietly invalidated the first.
func (s *Store) ResetBootstrap() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.doc.Users) > 0 {
		return "", fmt.Errorf("an operator account already exists; the bootstrap is retired")
	}
	return s.mintBootstrapLocked()
}

func (s *Store) mintBootstrapLocked() (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	previous := s.doc.BootstrapHash
	s.doc.BootstrapHash = hex.EncodeToString(hashToken(token))
	if err := s.persistLocked(); err != nil {
		s.doc.BootstrapHash = previous
		return "", err
	}
	return token, nil
}

// BootstrapLive reports whether the one-shot credential is still waiting to be
// used. The panel shows that as "create the first operator".
func (s *Store) BootstrapLive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.doc.Users) == 0 && s.doc.BootstrapHash != ""
}

// CreateFirstUser consumes the bootstrap token and creates the first admin.
func (s *Store) CreateFirstUser(bootstrap, username, password string) (User, error) {
	name, err := NormalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	if err := checkPassword(password); err != nil {
		return User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.doc.Users) > 0 {
		return User{}, fmt.Errorf("an operator account already exists")
	}
	if len(s.doc.BootstrapHash) != sha256.Size*2 {
		return User{}, fmt.Errorf("bootstrap is not live; run `relkit-agent admin bootstrap` on the server")
	}
	presented, err := hex.DecodeString(s.doc.BootstrapHash)
	if err != nil {
		return User{}, fmt.Errorf("bootstrapHash is not a sha256 hex digest")
	}
	if subtle.ConstantTimeCompare(hashToken(strings.TrimSpace(bootstrap)), presented) != 1 {
		return User{}, fmt.Errorf("invalid bootstrap")
	}
	user := User{
		Username:     name,
		PasswordHash: string(hash),
		Role:         RoleAdmin,
		CreatedAt:    s.now().UTC().Format(time.RFC3339),
	}
	previous := s.doc
	s.doc.Users = append(s.doc.Users, user)
	s.doc.BootstrapHash = ""
	if err := s.persistLocked(); err != nil {
		s.doc = previous
		return User{}, err
	}
	return user, nil
}

// Authenticate checks a username and password. It always runs one bcrypt
// comparison so a missing account costs the same time as a wrong password.
func (s *Store) Authenticate(username, password string) (User, bool) {
	name, err := NormalizeUsername(username)
	if err != nil {
		name = ""
	}
	s.mu.Lock()
	var found User
	for _, user := range s.doc.Users {
		if user.Username == name {
			found = user
			break
		}
	}
	s.mu.Unlock()
	hash := found.PasswordHash
	if hash == "" {
		hash = dummyHash()
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, false
	}
	if found.PasswordHash == "" {
		return User{}, false
	}
	return found, true
}

var (
	dummyOnce sync.Once
	dummy     []byte
)

func dummyHash() string {
	dummyOnce.Do(func() {
		dummy, _ = bcrypt.GenerateFromPassword([]byte("relkit-timing-dummy"), bcrypt.DefaultCost)
	})
	return string(dummy)
}

// Users returns every account. Password hashes are stripped: nothing outside
// this package needs them, and the API must never echo them.
func (s *Store) Users() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]User, 0, len(s.doc.Users))
	for _, user := range s.doc.Users {
		user.PasswordHash = ""
		user.Products = append([]string(nil), user.Products...)
		out = append(out, user)
	}
	return out
}

// User returns one account by name, hash stripped.
func (s *Store) User(username string) (User, bool) {
	name, err := NormalizeUsername(username)
	if err != nil {
		return User{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, user := range s.doc.Users {
		if user.Username == name {
			user.PasswordHash = ""
			user.Products = append([]string(nil), user.Products...)
			return user, true
		}
	}
	return User{}, false
}

// AddUser creates a non-bootstrap account.
func (s *Store) AddUser(username, password string, role Role, products []string) (User, error) {
	name, err := NormalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	if err := checkPassword(password); err != nil {
		return User{}, err
	}
	if role != RoleAdmin && role != RoleOwner {
		return User{}, fmt.Errorf("role %q is not %q or %q", role, RoleAdmin, RoleOwner)
	}
	assigned, err := s.checkProducts(products)
	if err != nil {
		return User{}, err
	}
	if role == RoleAdmin {
		assigned = nil
	} else if len(assigned) == 0 {
		return User{}, fmt.Errorf("a product owner needs at least one product")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, user := range s.doc.Users {
		if user.Username == name {
			return User{}, fmt.Errorf("user %q already exists", name)
		}
	}
	user := User{
		Username:     name,
		PasswordHash: string(hash),
		Role:         role,
		Products:     assigned,
		CreatedAt:    s.now().UTC().Format(time.RFC3339),
	}
	s.doc.Users = append(s.doc.Users, user)
	if err := s.persistLocked(); err != nil {
		s.doc.Users = s.doc.Users[:len(s.doc.Users)-1]
		return User{}, err
	}
	user.PasswordHash = ""
	return user, nil
}

// SetPassword replaces one account's password. Existing sessions stay valid for
// their remaining lifetime: a password change locks out new logins, not the
// operator who is already signed in.
func (s *Store) SetPassword(username, password string) error {
	name, err := NormalizeUsername(username)
	if err != nil {
		return err
	}
	if err := checkPassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.doc.Users {
		if s.doc.Users[index].Username != name {
			continue
		}
		previous := s.doc.Users[index].PasswordHash
		s.doc.Users[index].PasswordHash = string(hash)
		if err := s.persistLocked(); err != nil {
			s.doc.Users[index].PasswordHash = previous
			return err
		}
		return nil
	}
	return fmt.Errorf("user %q does not exist", name)
}

// SetUserProducts replaces a product owner's assignments.
func (s *Store) SetUserProducts(username string, products []string) (User, error) {
	name, err := NormalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	assigned, err := s.checkProducts(products)
	if err != nil {
		return User{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.doc.Users {
		if s.doc.Users[index].Username != name {
			continue
		}
		if s.doc.Users[index].Role != RoleOwner {
			return User{}, fmt.Errorf("user %q is an admin and already covers every product", name)
		}
		if len(assigned) == 0 {
			return User{}, fmt.Errorf("a product owner needs at least one product")
		}
		previous := s.doc.Users[index].Products
		s.doc.Users[index].Products = assigned
		if err := s.persistLocked(); err != nil {
			s.doc.Users[index].Products = previous
			return User{}, err
		}
		user := s.doc.Users[index]
		user.PasswordHash = ""
		user.Products = append([]string(nil), assigned...)
		return user, nil
	}
	return User{}, fmt.Errorf("user %q does not exist", name)
}

// SetRole moves an account between the two roles. It is a separate operation
// rather than a field of SetUserProducts because the two directions carry
// different obligations: demoting an admin means the products it will own must
// be named, and promoting an owner drops its assignments (an admin covers
// everything). The last admin cannot be demoted, for the same reason it cannot
// be deleted: nobody would be left able to manage users.
func (s *Store) SetRole(username string, role Role, products []string) (User, error) {
	name, err := NormalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	if role != RoleAdmin && role != RoleOwner {
		return User{}, fmt.Errorf("role %q is not %q or %q", role, RoleAdmin, RoleOwner)
	}
	assigned, err := s.checkProducts(products)
	if err != nil {
		return User{}, err
	}
	if role == RoleOwner && len(assigned) == 0 {
		return User{}, fmt.Errorf("a product owner needs at least one product")
	}
	if role == RoleAdmin {
		assigned = nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for i, user := range s.doc.Users {
		if user.Username == name {
			index = i
			break
		}
	}
	if index < 0 {
		return User{}, fmt.Errorf("user %q does not exist", name)
	}
	if role != RoleAdmin && s.doc.Users[index].Role == RoleAdmin {
		admins := 0
		for _, user := range s.doc.Users {
			if user.Role == RoleAdmin {
				admins++
			}
		}
		if admins == 1 {
			return User{}, fmt.Errorf("refusing to demote the last admin: nobody could manage users afterwards")
		}
	}
	previousRole := s.doc.Users[index].Role
	previousProducts := s.doc.Users[index].Products
	s.doc.Users[index].Role = role
	s.doc.Users[index].Products = assigned
	if err := s.persistLocked(); err != nil {
		s.doc.Users[index].Role = previousRole
		s.doc.Users[index].Products = previousProducts
		return User{}, err
	}
	user := s.doc.Users[index]
	user.PasswordHash = ""
	user.Products = append([]string(nil), assigned...)
	return user, nil
}

// DeleteUser removes an account. The last admin cannot be deleted: a server
// with no admin has no way back in except SSH and a fresh bootstrap.
func (s *Store) DeleteUser(username string) error {
	name, err := NormalizeUsername(username)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for i, user := range s.doc.Users {
		if user.Username == name {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("user %q does not exist", name)
	}
	if s.doc.Users[index].Role == RoleAdmin {
		admins := 0
		for _, user := range s.doc.Users {
			if user.Role == RoleAdmin {
				admins++
			}
		}
		if admins == 1 {
			return fmt.Errorf("refusing to delete the last admin: nobody could sign in afterwards")
		}
	}
	previous := s.doc.Users
	s.doc.Users = append(append([]User(nil), s.doc.Users[:index]...), s.doc.Users[index+1:]...)
	if err := s.persistLocked(); err != nil {
		s.doc.Users = previous
		return err
	}
	s.dropSessionsLocked(name)
	return nil
}

// Products returns the registered products sorted by id.
func (s *Store) Products() []Product {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Product, len(s.doc.Products))
	copy(out, s.doc.Products)
	sortProducts(out)
	return out
}

// Product returns one registration.
func (s *Store) Product(id string) (Product, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, product := range s.doc.Products {
		if product.ID == id {
			return product, true
		}
	}
	return Product{}, false
}

// AddProduct registers a product tree. Registration is the only thing that
// makes a product exist on this server; there is no config-file spelling.
func (s *Store) AddProduct(id, root, profile string) (Product, error) {
	if err := model.CheckIdentifier(id, "product"); err != nil {
		return Product{}, err
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return Product{}, fmt.Errorf("root is required")
	}
	product := Product{
		ID:        id,
		Root:      root,
		Profile:   strings.TrimSpace(profile),
		CreatedAt: s.now().UTC().Format(time.RFC3339),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.doc.Products {
		if existing.ID == id {
			return Product{}, fmt.Errorf("product %q is already registered", id)
		}
	}
	s.doc.Products = append(s.doc.Products, product)
	if err := s.persistLocked(); err != nil {
		s.doc.Products = s.doc.Products[:len(s.doc.Products)-1]
		return Product{}, err
	}
	return product, nil
}

// RemoveProduct forgets a registration and every token issued for it. The
// caller retires the published assets first: the registration is what the read
// face uses to answer, so it must outlive the deletion.
func (s *Store) RemoveProduct(id string) (removedTokens int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for i, product := range s.doc.Products {
		if product.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return 0, fmt.Errorf("product %q is not registered", id)
	}
	previousProducts := s.doc.Products
	previousTokens := s.doc.Tokens
	kept := make([]Token, 0, len(s.doc.Tokens))
	for _, token := range s.doc.Tokens {
		if token.Product == id {
			removedTokens++
			continue
		}
		kept = append(kept, token)
	}
	s.doc.Products = append(append([]Product(nil), s.doc.Products[:index]...), s.doc.Products[index+1:]...)
	s.doc.Tokens = kept
	if err := s.persistLocked(); err != nil {
		s.doc.Products = previousProducts
		s.doc.Tokens = previousTokens
		return 0, err
	}
	return removedTokens, nil
}

// Tokens returns every token, revoked ones included. Hashes are stripped so the
// listing can be shown and logged.
func (s *Store) Tokens(product string) []Token {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Token, 0, len(s.doc.Tokens))
	for _, token := range s.doc.Tokens {
		if product != "" && token.Product != product {
			continue
		}
		token.Hash = ""
		out = append(out, token)
	}
	return out
}

// Token returns one token by id, hash stripped.
func (s *Store) Token(id string) (Token, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, token := range s.doc.Tokens {
		if token.ID == id {
			token.Hash = ""
			return token, true
		}
	}
	return Token{}, false
}

// IssueToken mints a credential for one registered product and returns the
// plaintext once. Only the hash is kept (requirement 11.3).
func (s *Store) IssueToken(product, label string) (Token, string, error) {
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return Token{}, "", err
	}
	plaintext, err := randomToken()
	if err != nil {
		return Token{}, "", err
	}
	id, err := randomID()
	if err != nil {
		return Token{}, "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	registered := false
	for _, candidate := range s.doc.Products {
		if candidate.ID == product {
			registered = true
			break
		}
	}
	if !registered {
		return Token{}, "", fmt.Errorf("product %q is not registered", product)
	}
	token := Token{
		ID:        id,
		Product:   product,
		Label:     strings.TrimSpace(label),
		Hash:      hex.EncodeToString(hashToken(plaintext)),
		CreatedAt: s.now().UTC().Format(time.RFC3339),
	}
	s.doc.Tokens = append(s.doc.Tokens, token)
	if err := s.persistLocked(); err != nil {
		s.doc.Tokens = s.doc.Tokens[:len(s.doc.Tokens)-1]
		return Token{}, "", err
	}
	token.Hash = ""
	return token, plaintext, nil
}

// RotateToken replaces a token's secret and keeps its id, so the panel can show
// "this credential, rotated". The old value stops working the moment this
// returns.
func (s *Store) RotateToken(id string) (Token, string, error) {
	plaintext, err := randomToken()
	if err != nil {
		return Token{}, "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.doc.Tokens {
		if s.doc.Tokens[index].ID != id {
			continue
		}
		if s.doc.Tokens[index].Revoked() {
			return Token{}, "", fmt.Errorf("token %q is revoked; issue a new one", id)
		}
		previousHash := s.doc.Tokens[index].Hash
		previousRotated := s.doc.Tokens[index].RotatedAt
		s.doc.Tokens[index].Hash = hex.EncodeToString(hashToken(plaintext))
		s.doc.Tokens[index].RotatedAt = s.now().UTC().Format(time.RFC3339)
		if err := s.persistLocked(); err != nil {
			s.doc.Tokens[index].Hash = previousHash
			s.doc.Tokens[index].RotatedAt = previousRotated
			return Token{}, "", err
		}
		token := s.doc.Tokens[index]
		token.Hash = ""
		return token, plaintext, nil
	}
	return Token{}, "", fmt.Errorf("token %q does not exist", id)
}

// RevokeToken kills a credential without deleting the record.
func (s *Store) RevokeToken(id string) (Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.doc.Tokens {
		if s.doc.Tokens[index].ID != id {
			continue
		}
		if s.doc.Tokens[index].Revoked() {
			return Token{}, fmt.Errorf("token %q is already revoked", id)
		}
		s.doc.Tokens[index].RevokedAt = s.now().UTC().Format(time.RFC3339)
		if err := s.persistLocked(); err != nil {
			s.doc.Tokens[index].RevokedAt = ""
			return Token{}, err
		}
		token := s.doc.Tokens[index]
		token.Hash = ""
		return token, nil
	}
	return Token{}, fmt.Errorf("token %q does not exist", id)
}

// Credential is what the request path compares against: one hash and the one
// product it may operate.
type Credential struct {
	Hash    []byte
	Product string
}

// Allows reports whether this credential may act on product. A token names one
// product and never a set, so there is no widening rule to get wrong.
func (c Credential) Allows(product string) bool {
	return c.Product != "" && c.Product == product
}

// Credentials returns the live hashes the write face accepts.
func (s *Store) Credentials() []Credential {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Credential, 0, len(s.doc.Tokens))
	for _, token := range s.doc.Tokens {
		if token.Revoked() {
			continue
		}
		hash, err := hex.DecodeString(token.Hash)
		if err != nil || len(hash) != sha256.Size {
			continue
		}
		out = append(out, Credential{Hash: hash, Product: token.Product})
	}
	return out
}

// NewSession issues an opaque bearer for the management API. Sessions live in
// memory only: a restart signs everyone out, which is the safe default for a
// surface that can mint credentials.
func (s *Store) NewSession(username string, ttl time.Duration) (string, time.Time, error) {
	name, err := NormalizeUsername(username)
	if err != nil {
		return "", time.Time{}, err
	}
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	token, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := s.now().Add(ttl)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[hex.EncodeToString(hashToken(token))] = session{username: name, expiresAt: expires}
	return token, expires, nil
}

// SessionUser resolves a bearer to an account, dropping expired entries.
func (s *Store) SessionUser(token string) (User, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, false
	}
	key := hex.EncodeToString(hashToken(token))
	s.mu.Lock()
	entry, ok := s.sessions[key]
	if ok && !s.now().Before(entry.expiresAt) {
		delete(s.sessions, key)
		ok = false
	}
	var found User
	if ok {
		for _, user := range s.doc.Users {
			if user.Username == entry.username {
				found = user
				break
			}
		}
		if found.Username == "" {
			delete(s.sessions, key)
			ok = false
		}
	}
	s.mu.Unlock()
	if !ok {
		return User{}, false
	}
	found.PasswordHash = ""
	found.Products = append([]string(nil), found.Products...)
	return found, true
}

// DropSession forgets one bearer.
func (s *Store) DropSession(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	s.mu.Lock()
	delete(s.sessions, hex.EncodeToString(hashToken(token)))
	s.mu.Unlock()
}

func (s *Store) dropSessionsLocked(username string) {
	for key, entry := range s.sessions {
		if entry.username == username {
			delete(s.sessions, key)
		}
	}
}

func (s *Store) checkProducts(products []string) ([]string, error) {
	out := make([]string, 0, len(products))
	seen := map[string]bool{}
	for _, id := range products {
		if err := model.CheckIdentifier(id, "product"); err != nil {
			return nil, err
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range out {
		registered := false
		for _, product := range s.doc.Products {
			if product.ID == id {
				registered = true
				break
			}
		}
		if !registered {
			return nil, fmt.Errorf("product %q is not registered", id)
		}
	}
	return out, nil
}

// NormalizeUsername lowercases and validates an account name.
func NormalizeUsername(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if len(name) < 2 || len(name) > 32 {
		return "", fmt.Errorf("username must be 2-32 characters")
	}
	for _, r := range name {
		ok := r == '.' || r == '_' || r == '-' ||
			(r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !ok {
			return "", fmt.Errorf("username may contain letters, digits, '.', '_' and '-'")
		}
	}
	return name, nil
}

func checkPassword(password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}
	return nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func randomID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "tok_" + hex.EncodeToString(buf), nil
}

func sortProducts(products []Product) {
	for i := 1; i < len(products); i++ {
		for j := i; j > 0 && products[j].ID < products[j-1].ID; j-- {
			products[j], products[j-1] = products[j-1], products[j]
		}
	}
}
