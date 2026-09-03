package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/directory"
	"cnb.cool/shichao402/relkit/internal/model"
	"cnb.cool/shichao402/relkit/internal/publish"
	"cnb.cool/shichao402/relkit/internal/stage"
)

const errInstanceToken = "instance-wide agent tokens are gone: delete uploadToken, uploadTokenFile, and RELKIT_AGENT_TOKEN; issue one token per product with `relkit-agent init -product <id>` (CI env is RELKIT_UPLOAD_TOKEN)"

type ProductConfig struct {
	Root    string `json:"root"`
	Profile string `json:"profile,omitempty"`
}

type FileConfig struct {
	Addr            string                   `json:"addr"`
	UploadToken     string                   `json:"uploadToken,omitempty"`
	UploadTokenFile string                   `json:"uploadTokenFile,omitempty"`
	UploadTokens    []UploadTokenEntry       `json:"uploadTokens,omitempty"`
	MaxUpload       string                   `json:"maxUpload,omitempty"`
	MaxFiles        int                      `json:"maxFiles,omitempty"`
	StateDir        string                   `json:"stateDir,omitempty"`
	Products        map[string]ProductConfig `json:"products"`
}

// UploadTokenEntry is a product-scoped publisher credential. One file, one
// product. Sharing a file across products is refused at load time.
type UploadTokenEntry struct {
	File     string   `json:"file"`
	Products []string `json:"products"`
}

type credential struct {
	hash     []byte
	products []string
}

func (c credential) allows(product string) bool {
	for _, id := range c.products {
		if id == product {
			return true
		}
	}
	return false
}

type Config struct {
	Addr        string
	credentials []credential
	MaxUpload   int64
	MaxFiles    int
	StateDir    string
	Products    map[string]ProductConfig
	ConfigPath  string
}

func LoadConfig(path string) (*Config, error) {
	raw, err := loadFileConfig(path)
	if err != nil {
		return nil, err
	}
	if raw.Products == nil {
		raw.Products = map[string]ProductConfig{}
	}
	cfg := &Config{
		Addr:       raw.Addr,
		MaxUpload:  4 << 30,
		MaxFiles:   10_000,
		StateDir:   raw.StateDir,
		Products:   raw.Products,
		ConfigPath: path,
	}
	if raw.MaxFiles > 0 {
		cfg.MaxFiles = raw.MaxFiles
	}
	if raw.MaxUpload != "" {
		n, err := parseSize(raw.MaxUpload)
		if err != nil {
			return nil, err
		}
		cfg.MaxUpload = n
	}
	if cfg.StateDir == "" {
		cfg.StateDir = filepath.Join(filepath.Dir(path), "relkit-agent-state")
	}
	creds, err := loadProductTokens(path, raw)
	if err != nil {
		return nil, err
	}
	cfg.credentials = creds
	for name, p := range cfg.Products {
		if strings.TrimSpace(p.Root) == "" {
			return nil, fmt.Errorf("product %q: root is required", name)
		}
		root := p.Root
		if !filepath.IsAbs(root) {
			root = filepath.Join(filepath.Dir(path), root)
		}
		p.Root = mustAbs(root)
		if strings.TrimSpace(p.Profile) == "" {
			p.Profile = filepath.Join(filepath.Dir(path), "products", name+".json")
		} else if !filepath.IsAbs(p.Profile) {
			p.Profile = filepath.Join(filepath.Dir(path), p.Profile)
		}
		p.Profile = mustAbs(p.Profile)
		cfg.Products[name] = p
	}
	return cfg, nil
}

func loadFileConfig(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw FileConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &raw, nil
}

func writeFileConfig(path string, cfg *FileConfig) error {
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

func (e UploadTokenEntry) validate() error {
	if strings.TrimSpace(e.File) == "" {
		return fmt.Errorf("file is required")
	}
	if len(e.Products) != 1 {
		return fmt.Errorf("products must list exactly one id (got %v); do not share a token across products", e.Products)
	}
	if err := model.CheckIdentifier(e.Products[0], "product"); err != nil {
		return err
	}
	return nil
}

func loadProductTokens(configPath string, raw *FileConfig) ([]credential, error) {
	if raw.UploadToken != "" || raw.UploadTokenFile != "" {
		return nil, fmt.Errorf("%s", errInstanceToken)
	}
	if strings.TrimSpace(os.Getenv("RELKIT_AGENT_TOKEN")) != "" {
		return nil, fmt.Errorf("%s", errInstanceToken)
	}
	var creds []credential
	seenFile := map[string]bool{}
	seenProduct := map[string]bool{}
	seenHash := map[string]bool{}
	for i, entry := range raw.UploadTokens {
		if err := entry.validate(); err != nil {
			return nil, fmt.Errorf("uploadTokens[%d]: %w", i, err)
		}
		product := entry.Products[0]
		if seenProduct[product] {
			return nil, fmt.Errorf("uploadTokens[%d]: duplicate product %q", i, product)
		}
		seenProduct[product] = true
		path := entry.File
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(configPath), path)
		}
		if seenFile[path] {
			return nil, fmt.Errorf("uploadTokens[%d]: token file already used by another product", i)
		}
		seenFile[path] = true
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("uploadTokens[%d]: %w", i, err)
		}
		token := strings.TrimSpace(string(data))
		if token == "" {
			return nil, fmt.Errorf("uploadTokens[%d]: %s is empty", i, path)
		}
		hash := hashToken(token)
		key := hex.EncodeToString(hash)
		if seenHash[key] {
			return nil, fmt.Errorf("uploadTokens[%d]: duplicate token hash (products must not share a secret)", i)
		}
		seenHash[key] = true
		creds = append(creds, credential{hash: hash, products: []string{product}})
	}
	return creds, nil
}

func parseSize(text string) (int64, error) {
	text = strings.TrimSpace(text)
	multipliers := map[string]int64{
		"b": 1, "kb": 1000, "mb": 1000 * 1000, "gb": 1000 * 1000 * 1000,
		"kib": 1024, "mib": 1024 * 1024, "gib": 1024 * 1024 * 1024,
	}
	lower := strings.ToLower(text)
	for suffix, mul := range multipliers {
		if strings.HasSuffix(lower, suffix) {
			num := strings.TrimSpace(text[:len(text)-len(suffix)])
			var n int64
			if _, err := fmt.Sscan(num, &n); err != nil {
				return 0, err
			}
			return n * mul, nil
		}
	}
	var n int64
	if _, err := fmt.Sscan(text, &n); err != nil {
		return 0, err
	}
	return n, nil
}

type Server struct {
	cfg    *Config
	locks  sync.Map // product -> *sync.Mutex
	idemMu sync.Mutex
}

func NewServer(cfg *Config) *Server {
	return &Server{cfg: cfg}
}

func (s *Server) productLock(product string) *sync.Mutex {
	v, _ := s.locks.LoadOrStore(product, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func (s *Server) lookupCredential(r *http.Request) *credential {
	header := r.Header.Get("Authorization")
	value, found := strings.CutPrefix(header, "Bearer ")
	if !found {
		return nil
	}
	presented := hashToken(strings.TrimSpace(value))
	var matched *credential
	for i := range s.cfg.credentials {
		if subtle.ConstantTimeCompare(presented, s.cfg.credentials[i].hash) == 1 {
			matched = &s.cfg.credentials[i]
		}
	}
	return matched
}

func (s *Server) requireAuthFor(w http.ResponseWriter, r *http.Request, product string) bool {
	if len(s.cfg.credentials) == 0 {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "uploads are disabled on this server", http.StatusMethodNotAllowed)
		return false
	}
	cred := s.lookupCredential(r)
	if cred == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-agent"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if !cred.allows(product) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version})
}

func (s *Server) handleStaged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	product, versionRaw, ok := parseStagedRoute(r.URL.Path)
	if !ok {
		http.Error(w, "expected /v1/staged/{product}/{version}", http.StatusBadRequest)
		return
	}
	if !s.requireAuthFor(w, r, product) {
		return
	}
	version, ok := cleanVersion(versionRaw)
	if !ok {
		http.Error(w, "invalid version", http.StatusBadRequest)
		return
	}
	pc, ok := s.cfg.Products[product]
	if !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return
	}

	body := http.MaxBytesReader(w, r.Body, s.cfg.MaxUpload)
	tmp, err := os.CreateTemp("", "relkit-staged-*.tar.gz")
	if err != nil {
		http.Error(w, "temp file", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), body)
	closeErr := tmp.Close()
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	if closeErr != nil {
		http.Error(w, "temp close failed", http.StatusInternalServerError)
		return
	}
	sum := hex.EncodeToString(h.Sum(nil))

	mu := s.productLock(product)
	if !mu.TryLock() {
		http.Error(w, "publish or staged upload already in progress for this product", http.StatusConflict)
		return
	}
	defer mu.Unlock()

	dest := stage.StagingDir(pc.Root, version)
	_ = os.RemoveAll(dest)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		http.Error(w, "mkdir staged", http.StatusInternalServerError)
		return
	}
	if err := extractTarGz(tmpPath, dest, s.cfg.MaxFiles); err != nil {
		_ = os.RemoveAll(dest)
		log.Printf("staged extract %s/%s: %v", product, version, err)
		http.Error(w, "extract: "+err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := stage.LoadStaged(pc.Root, version); err != nil {
		_ = os.RemoveAll(dest)
		log.Printf("staged tree %s/%s: %v", product, version, err)
		http.Error(w, "invalid staged tree: "+err.Error(), http.StatusBadRequest)
		return
	}
	if _, statErr := os.Stat(stage.ReleasePolicyPath(pc.Root, version)); statErr == nil {
		policy, err := stage.LoadReleasePolicy(pc.Root, version)
		if err != nil {
			_ = os.RemoveAll(dest)
			http.Error(w, "invalid release policy: "+err.Error(), http.StatusBadRequest)
			return
		}
		if policy.Product != product {
			_ = os.RemoveAll(dest)
			http.Error(w, fmt.Sprintf("release policy product %q does not match route product %q", policy.Product, product), http.StatusBadRequest)
			return
		}
	} else if !os.IsNotExist(statErr) {
		_ = os.RemoveAll(dest)
		http.Error(w, "inspect release policy: "+statErr.Error(), http.StatusBadRequest)
		return
	}
	if err := s.writeStagedSHA(product, version, sum); err != nil {
		_ = os.RemoveAll(dest)
		http.Error(w, "persist staged sha256: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"product": product,
		"version": version,
		"bytes":   n,
		"sha256":  sum,
		"path":    dest,
	})
}

// parseStagedRoute accepts /v1/staged/{product}/{version}, including a
// doubled slash when RELKIT_PUBLISH_URL was configured with a trailing slash.
func parseStagedRoute(urlPath string) (product, version string, ok bool) {
	cleaned := path.Clean("/" + strings.TrimSpace(urlPath))
	rest := strings.TrimPrefix(cleaned, "/v1/staged/")
	if rest == cleaned {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (s *Server) stagedSHAPath(product, version string) string {
	return filepath.Join(s.cfg.StateDir, "staged", product, version+".sha256")
}

func (s *Server) writeStagedSHA(product, version, sum string) error {
	path := s.stagedSHAPath(product, version)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sha256-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.WriteString(tmp, strings.ToLower(sum)+"\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Remove(path) // Windows cannot rename over an existing file.
	return os.Rename(tmpPath, path)
}

func (s *Server) readStagedSHA(product, version string) (string, error) {
	data, err := os.ReadFile(s.stagedSHAPath(product, version))
	if err != nil {
		return "", err
	}
	sum := strings.TrimSpace(strings.ToLower(string(data)))
	if !validSHA256Hex(sum) {
		return "", fmt.Errorf("invalid stored sha256")
	}
	return sum, nil
}

func validSHA256Hex(sum string) bool {
	if len(sum) != sha256.Size*2 {
		return false
	}
	for i := 0; i < len(sum); i++ {
		c := sum[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func sha256HexEqual(presented, stored string) bool {
	a := strings.ToLower(strings.TrimSpace(presented))
	b := strings.ToLower(strings.TrimSpace(stored))
	if !validSHA256Hex(a) || !validSHA256Hex(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func loadProductConfig(pc ProductConfig, version string) (*config.Config, string, error) {
	policyPath := stage.ReleasePolicyPath(pc.Root, version)
	policy, err := config.LoadProductPolicy(policyPath)
	if err != nil {
		return nil, "", fmt.Errorf("release-policy.json required at %s: %w", policyPath, err)
	}
	if strings.TrimSpace(pc.Profile) == "" {
		return nil, "", fmt.Errorf("publish profile path is empty for product %q", policy.Product)
	}
	profile, err := config.LoadPublishProfile(pc.Profile)
	if err != nil {
		return nil, "", fmt.Errorf("profile %s: %w", pc.Profile, err)
	}
	cfg, err := config.MergeProductPolicy(policy, profile, pc.Root)
	if err != nil {
		return nil, "", err
	}
	return cfg, "staged release-policy.json + " + pc.Profile, nil
}

type publishRequest struct {
	Product        string   `json:"product"`
	Version        string   `json:"version"`
	To             []string `json:"to"`
	DryRun         bool     `json:"dryRun"`
	AllowBackfill  bool     `json:"allowBackfill"`
	AllowPartial   bool     `json:"allowPartial"`
	IdempotencyKey string   `json:"idempotencyKey"`
	StagedSHA256   string   `json:"stagedSha256"`
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var req publishRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	version, ok := cleanVersion(req.Version)
	if !ok || req.Product == "" {
		http.Error(w, "product and version required", http.StatusBadRequest)
		return
	}
	if !s.requireAuthFor(w, r, req.Product) {
		return
	}
	pc, ok := s.cfg.Products[req.Product]
	if !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return
	}
	if req.StagedSHA256 != "" {
		stored, err := s.readStagedSHA(req.Product, version)
		if err != nil {
			http.Error(w, "load staged sha256: "+err.Error(), http.StatusConflict)
			return
		}
		if !sha256HexEqual(req.StagedSHA256, stored) {
			http.Error(w, "staged sha256 mismatch; re-upload", http.StatusConflict)
			return
		}
	}

	idemKey := req.IdempotencyKey
	if idemKey == "" && req.StagedSHA256 != "" {
		idemKey = req.Product + "/" + version + "/" + req.StagedSHA256
	}
	if idemKey != "" {
		if cached, ok := s.loadIdempotent(idemKey); ok {
			writeJSON(w, http.StatusOK, cached)
			return
		}
	}

	mu := s.productLock(req.Product)
	if !mu.TryLock() {
		http.Error(w, "publish already in progress for this product", http.StatusConflict)
		return
	}
	defer mu.Unlock()

	cfg, source, err := loadProductConfig(pc, version)
	if err != nil {
		http.Error(w, "load publish config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if cfg.Product != "" && cfg.Product != req.Product {
		http.Error(w, "product mismatch with publish config", http.StatusBadRequest)
		return
	}

	staged, err := stage.LoadStaged(cfg.Root, version)
	if err != nil {
		http.Error(w, "load staged: "+err.Error(), http.StatusBadRequest)
		return
	}
	if mismatches := stage.VerifyStagedHashes(cfg, staged); len(mismatches) > 0 {
		http.Error(w, "staged hash mismatch; re-upload", http.StatusConflict)
		return
	}

	var logs []string
	logs = append(logs, "publish config: "+source)
	printer := func(line string) { logs = append(logs, line) }
	index, err := publish.Run(cfg, version, req.To, req.DryRun, req.AllowBackfill, req.AllowPartial, printer)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error(), "log": logs})
		return
	}
	if !req.DryRun && cfg.Directory != nil {
		if _, dirErr := directory.Set(cfg, directory.Options{To: req.To}, printer); dirErr != nil {
			logs = append(logs, "directory: "+dirErr.Error())
		}
	}
	resp := map[string]any{
		"ok":      true,
		"product": req.Product,
		"version": version,
		"dryRun":  req.DryRun,
		"log":     logs,
	}
	if index != nil {
		resp["sequence"] = index.Sequence
		resp["channel"] = index.Channel
	}
	if idemKey != "" && !req.DryRun {
		_ = s.saveIdempotent(idemKey, resp)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) idemPath(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.cfg.StateDir, "idempotency", hex.EncodeToString(sum[:])+".json")
}

func (s *Server) loadIdempotent(key string) (map[string]any, bool) {
	s.idemMu.Lock()
	defer s.idemMu.Unlock()
	data, err := os.ReadFile(s.idemPath(key))
	if err != nil {
		return nil, false
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, false
	}
	out["idempotentReplay"] = true
	return out, true
}

func (s *Server) saveIdempotent(key string, resp map[string]any) error {
	s.idemMu.Lock()
	defer s.idemMu.Unlock()
	path := s.idemPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
