package main

// The panel is a client of the agent's management API, not a second admin
// implementation. Requirement 13.6 is explicit: writes travel through the agent
// and the panel process never touches agent storage. Everything in this file is
// therefore HTTP with no file path in it.
//
// The division of labour follows from that. The agent owns the document (users,
// roles, products, token hashes); the panel owns rendering and the sign-in
// cookie that carries the session bearer. A console that loses its own state
// file loses nothing but its cookie signing key.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// agentUser mirrors agentadmin.User over the wire. PasswordHash is never sent:
// the agent strips it, and this struct has no field to receive it even if it did.
type agentUser struct {
	Username  string   `json:"username"`
	Role      string   `json:"role"`
	Products  []string `json:"products,omitempty"`
	CreatedAt string   `json:"createdAt"`
}

// agentProduct mirrors agentadmin.Product over the wire.
type agentProduct struct {
	ID        string `json:"id"`
	Root      string `json:"root"`
	Profile   string `json:"profile,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// agentProductView is one row of GET /v1/admin/products: the registration plus
// the paste-able block. The block carries no secret (requirement 13.4).
type agentProductView struct {
	Product       agentProduct   `json:"product"`
	ConnectConfig map[string]any `json:"connectConfig"`
}

func (v agentProductView) server() string      { return connectString(v.ConnectConfig, "server") }
func (v agentProductView) rootKey() string     { return connectString(v.ConnectConfig, "rootPublicKey") }
func (v agentProductView) rootKeyID() string   { return connectString(v.ConnectConfig, "rootKeyId") }
func (v agentProductView) fingerprint() string { return connectString(v.ConnectConfig, "rootFingerprint") }

func connectString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	value, ok := config[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

// agentToken mirrors agentadmin.Token over the wire. Hashes never travel.
type agentToken struct {
	ID        string `json:"id"`
	Product   string `json:"product"`
	Label     string `json:"label,omitempty"`
	CreatedAt string `json:"createdAt"`
	RotatedAt string `json:"rotatedAt,omitempty"`
	RevokedAt string `json:"revokedAt,omitempty"`
}

// Revoked reports whether this credential is dead. A revoked token stays in the
// listing so the panel can show what was withdrawn and when.
func (t agentToken) Revoked() bool { return t.RevokedAt != "" }

type agentStatus struct {
	BootstrapLive  bool
	OperatorExists bool
}

type agentLogin struct {
	Session   string
	User      agentUser
	ExpiresAt string
}

// issuedToken is the one response that carries a plaintext credential. It is
// rendered exactly once and then dropped (requirement 11.3).
type issuedToken struct {
	Token     agentToken
	Plaintext string
}

// agentError is a refusal from the agent. StatusCode matters because 401 means
// "sign in again" while everything else is a message to show verbatim.
type agentError struct {
	StatusCode int
	Message    string
}

func (e *agentError) Error() string { return e.Message }

func (e *agentError) Unauthorized() bool {
	return e.StatusCode == http.StatusUnauthorized
}

// agentClient talks to one agent server. base is the server root, so the panel
// config holds the same address a host project would (requirement 13.4).
type agentClient struct {
	base string
	http *http.Client
}

func newAgentClient(base string) (*agentClient, error) {
	trimmed := strings.TrimSpace(base)
	if trimmed == "" {
		return nil, fmt.Errorf("agent URL is not configured")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("agent URL %q is not an absolute http(s) URL", base)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("agent URL %q must be http or https", base)
	}
	return &agentClient{
		base: strings.TrimSuffix(parsed.String(), "/"),
		http: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *agentClient) do(method, path, session string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("agent unreachable at %s: %w", c.base, err)
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(raw))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return &agentError{StatusCode: resp.StatusCode, Message: message}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *agentClient) Status() (agentStatus, error) {
	var payload struct {
		BootstrapLive  bool `json:"bootstrapLive"`
		OperatorExists bool `json:"operatorExists"`
	}
	if err := c.do(http.MethodGet, "/v1/admin/status", "", nil, &payload); err != nil {
		return agentStatus{}, err
	}
	return agentStatus{
		BootstrapLive:  payload.BootstrapLive,
		OperatorExists: payload.OperatorExists,
	}, nil
}

func (c *agentClient) Bootstrap(bootstrap, username, password string) (agentLogin, error) {
	return c.login("/v1/admin/bootstrap", map[string]string{
		"bootstrap": bootstrap,
		"username":  username,
		"password":  password,
	})
}

func (c *agentClient) Login(username, password string) (agentLogin, error) {
	return c.login("/v1/admin/login", map[string]string{
		"username": username,
		"password": password,
	})
}

func (c *agentClient) login(path string, body any) (agentLogin, error) {
	var payload struct {
		Session   string    `json:"session"`
		User      agentUser `json:"user"`
		ExpiresAt string    `json:"expiresAt"`
	}
	if err := c.do(http.MethodPost, path, "", body, &payload); err != nil {
		return agentLogin{}, err
	}
	if payload.Session == "" {
		return agentLogin{}, fmt.Errorf("agent returned no session")
	}
	return agentLogin{Session: payload.Session, User: payload.User, ExpiresAt: payload.ExpiresAt}, nil
}

func (c *agentClient) Logout(session string) error {
	return c.do(http.MethodPost, "/v1/admin/logout", session, nil, nil)
}

func (c *agentClient) Whoami(session string) (agentUser, error) {
	var payload struct {
		User agentUser `json:"user"`
	}
	if err := c.do(http.MethodGet, "/v1/admin/whoami", session, nil, &payload); err != nil {
		return agentUser{}, err
	}
	return payload.User, nil
}

func (c *agentClient) Users(session string) ([]agentUser, error) {
	var payload struct {
		Users []agentUser `json:"users"`
	}
	if err := c.do(http.MethodGet, "/v1/admin/users", session, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Users, nil
}

func (c *agentClient) AddUser(session, username, password, role string, products []string) (agentUser, error) {
	var payload struct {
		User agentUser `json:"user"`
	}
	body := map[string]any{
		"username": username,
		"password": password,
		"role":     role,
		"products": products,
	}
	if err := c.do(http.MethodPost, "/v1/admin/users", session, body, &payload); err != nil {
		return agentUser{}, err
	}
	return payload.User, nil
}

func (c *agentClient) DeleteUser(session, username string) error {
	return c.do(http.MethodDelete, "/v1/admin/users/"+url.PathEscape(username), session, nil, nil)
}

func (c *agentClient) SetPassword(session, username, password string) error {
	body := map[string]string{"password": password}
	return c.do(http.MethodPost, "/v1/admin/users/"+url.PathEscape(username)+"/password", session, body, nil)
}

// SetRole carries the product assignment in the same call: demoting an admin
// means naming the products it will own, so the agent refuses a role change that
// does not say which (requirement 13.3).
func (c *agentClient) SetRole(session, username, role string, products []string) (agentUser, error) {
	var payload struct {
		User agentUser `json:"user"`
	}
	body := map[string]any{"role": role, "products": products}
	if err := c.do(http.MethodPost, "/v1/admin/users/"+url.PathEscape(username)+"/role", session, body, &payload); err != nil {
		return agentUser{}, err
	}
	return payload.User, nil
}

func (c *agentClient) SetUserProducts(session, username string, products []string) (agentUser, error) {
	var payload struct {
		User agentUser `json:"user"`
	}
	body := map[string]any{"products": products}
	if err := c.do(http.MethodPut, "/v1/admin/users/"+url.PathEscape(username)+"/products", session, body, &payload); err != nil {
		return agentUser{}, err
	}
	return payload.User, nil
}

func (c *agentClient) Products(session string) ([]agentProductView, error) {
	var payload struct {
		Products []agentProductView `json:"products"`
	}
	if err := c.do(http.MethodGet, "/v1/admin/products", session, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Products, nil
}

// AddProduct registers the product and returns the first token's plaintext,
// which the agent shows exactly once (requirements 13.4, 11.3).
func (c *agentClient) AddProduct(session, id, root, profile, label string) (map[string]any, issuedToken, error) {
	var payload map[string]any
	body := map[string]string{"id": id, "root": root, "profile": profile, "label": label}
	if err := c.do(http.MethodPost, "/v1/admin/products", session, body, &payload); err != nil {
		return nil, issuedToken{}, err
	}
	return payload, issuedToken{
		Token:     decodeToken(payload["token"]),
		Plaintext: decodeString(payload["plaintext"]),
	}, nil
}

func (c *agentClient) DeleteProduct(session, id string) (map[string]any, error) {
	var payload map[string]any
	if err := c.do(http.MethodDelete, "/v1/admin/products/"+url.PathEscape(id), session, nil, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *agentClient) Tokens(session, product string) ([]agentToken, error) {
	var payload struct {
		Tokens []agentToken `json:"tokens"`
	}
	path := "/v1/admin/products/" + url.PathEscape(product) + "/tokens"
	if err := c.do(http.MethodGet, path, session, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Tokens, nil
}

func (c *agentClient) IssueToken(session, product, label string) (issuedToken, error) {
	var payload map[string]any
	path := "/v1/admin/products/" + url.PathEscape(product) + "/tokens"
	body := map[string]string{"label": label}
	if err := c.do(http.MethodPost, path, session, body, &payload); err != nil {
		return issuedToken{}, err
	}
	return issuedToken{
		Token:     decodeToken(payload["token"]),
		Plaintext: decodeString(payload["plaintext"]),
	}, nil
}

func (c *agentClient) tokenAction(session, id, action string) (issuedToken, error) {
	var payload map[string]any
	path := "/v1/admin/tokens/" + url.PathEscape(id) + "/" + action
	if err := c.do(http.MethodPost, path, session, nil, &payload); err != nil {
		return issuedToken{}, err
	}
	return issuedToken{
		Token:     decodeToken(payload["token"]),
		Plaintext: decodeString(payload["plaintext"]),
	}, nil
}

func (c *agentClient) RotateToken(session, id string) (issuedToken, error) {
	return c.tokenAction(session, id, "rotate")
}

func (c *agentClient) RevokeToken(session, id string) (issuedToken, error) {
	return c.tokenAction(session, id, "revoke")
}

// decodeToken and decodeString unpack the union-typed maps the issuing
// endpoints return. They are typed helpers rather than structs because the
// create-product response mixes the registration, the token and the connect
// block, and a partial struct would silently drop whichever field a future
// agent version adds.
func decodeToken(raw any) agentToken {
	object, ok := raw.(map[string]any)
	if !ok {
		return agentToken{}
	}
	return agentToken{
		ID:        decodeString(object["id"]),
		Product:   decodeString(object["product"]),
		Label:     decodeString(object["label"]),
		CreatedAt: decodeString(object["createdAt"]),
		RotatedAt: decodeString(object["rotatedAt"]),
		RevokedAt: decodeString(object["revokedAt"]),
	}
}

func decodeString(raw any) string {
	text, _ := raw.(string)
	return text
}

