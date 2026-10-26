package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// apiClient drives the JSON API the way the front end does: a cookie jar,
// Origin on every write, and the CSRF token from the session answer.
type apiClient struct {
	t    *testing.T
	srv  *httptest.Server
	http *http.Client
	csrf string
}

func newAPIClient(t *testing.T, srv *httptest.Server) *apiClient {
	return &apiClient{t: t, srv: srv, http: jarClient(t, srv)}
}

func (a *apiClient) call(method, path string, body any, headers map[string]string) (int, map[string]any) {
	a.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, a.srv.URL+path, reader)
	if err != nil {
		a.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if !isReadMethod(method) {
		req.Header.Set("Origin", a.srv.URL)
		if a.csrf != "" {
			req.Header.Set(apiCSRFHeader, a.csrf)
		}
	}
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := a.http.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Cache-Control"); ct != "no-store" {
		a.t.Fatalf("%s %s: Cache-Control=%q, want no-store", method, path, ct)
	}
	var out map[string]any
	raw, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(raw, &out); err != nil {
		a.t.Fatalf("%s %s: non-JSON answer %d: %s", method, path, resp.StatusCode, raw)
	}
	return resp.StatusCode, out
}

func (a *apiClient) login() {
	a.t.Helper()
	status, body := a.call(http.MethodPost, apiPrefix+"session/login",
		map[string]string{"username": testPanelUser, "password": testPanelPassword}, nil)
	if status != http.StatusOK {
		a.t.Fatalf("login = %d %v", status, body)
	}
	a.csrf, _ = body["csrf"].(string)
	if a.csrf == "" {
		a.t.Fatalf("login answered no csrf: %v", body)
	}
}

func newAPITest(t *testing.T) (*apiClient, *agentStub) {
	t.Helper()
	api, stub, _ := newAPITestDir(t)
	return api, stub
}

func newAPITestDir(t *testing.T) (*apiClient, *agentStub, string) {
	t.Helper()
	dir := t.TempDir()
	adapter := newTreeAdapter(t, dir)
	stub, agentURL := newAgentStubServer(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	admin, err := openPanelSession(t.TempDir()+"/panel.json", agentURL)
	if err != nil {
		t.Fatal(err)
	}
	c := &console{adapter: adapter, admin: admin}
	srv := newLocalServer(t, c)
	return newAPIClient(t, srv), stub, dir
}

func TestAPISessionGateBeforeLogin(t *testing.T) {
	api, _ := newAPITest(t)
	status, body := api.call(http.MethodGet, apiPrefix+"session", nil, nil)
	if status != http.StatusOK || body["gate"] != "login" {
		t.Fatalf("session = %d %v, want gate=login", status, body)
	}
	if _, ok := body["csrf"]; ok {
		t.Fatalf("anonymous session must not carry csrf: %v", body)
	}
}

func TestAPISessionAfterLoginCarriesUserAndSettings(t *testing.T) {
	api, _ := newAPITest(t)
	api.login()
	status, body := api.call(http.MethodGet, apiPrefix+"session", nil, nil)
	if status != http.StatusOK || body["gate"] != "ready" {
		t.Fatalf("session = %d %v", status, body)
	}
	user, _ := body["user"].(map[string]any)
	if user["username"] != testPanelUser {
		t.Fatalf("user = %v", user)
	}
	settings, _ := body["settings"].(map[string]any)
	if settings["productRootParent"] != "/data/relkit" {
		t.Fatalf("settings = %v, want the agent's real parent", settings)
	}
}

func TestAPILoginWrongPassword(t *testing.T) {
	saved := authFailDelay
	authFailDelay = 0
	t.Cleanup(func() { authFailDelay = saved })
	api, _ := newAPITest(t)
	status, body := api.call(http.MethodPost, apiPrefix+"session/login",
		map[string]string{"username": "nobody", "password": "x"}, nil)
	if status != http.StatusUnauthorized || body["code"] != "invalid_credentials" {
		t.Fatalf("login = %d %v", status, body)
	}
}

func TestAPIUnauthenticatedIs401(t *testing.T) {
	api, _ := newAPITest(t)
	status, body := api.call(http.MethodGet, apiPrefix+"products", nil, nil)
	if status != http.StatusUnauthorized || body["code"] != "unauthenticated" {
		t.Fatalf("products = %d %v", status, body)
	}
}

func TestAPIWriteNeedsCSRF(t *testing.T) {
	api, _ := newAPITest(t)
	api.login()
	good := api.csrf
	api.csrf = ""
	status, body := api.call(http.MethodPost, apiPrefix+"session/logout", nil, nil)
	if status != http.StatusForbidden || body["code"] != "csrf" {
		t.Fatalf("logout without csrf = %d %v", status, body)
	}
	api.csrf = "forged"
	if status, _ := api.call(http.MethodPost, apiPrefix+"session/logout", nil, nil); status != http.StatusForbidden {
		t.Fatalf("logout with forged csrf = %d", status)
	}
	api.csrf = good
	if status, body := api.call(http.MethodPost, apiPrefix+"session/logout", nil, nil); status != http.StatusOK {
		t.Fatalf("logout = %d %v", status, body)
	}
	if _, body := api.call(http.MethodGet, apiPrefix+"session", nil, nil); body["gate"] != "login" {
		t.Fatalf("after logout gate = %v", body["gate"])
	}
}

func TestAPIWriteRefusesCrossOrigin(t *testing.T) {
	api, _ := newAPITest(t)
	status, body := api.call(http.MethodPost, apiPrefix+"session/login",
		map[string]string{"username": testPanelUser, "password": testPanelPassword},
		map[string]string{"Origin": "https://evil.example"})
	if status != http.StatusForbidden {
		t.Fatalf("cross-origin login = %d %v", status, body)
	}
	status, _ = api.call(http.MethodPost, apiPrefix+"session/login",
		map[string]string{"username": testPanelUser, "password": testPanelPassword},
		map[string]string{"Origin": ""})
	if status != http.StatusForbidden {
		t.Fatalf("login with no Origin = %d, want 403", status)
	}
}

func TestAPIWriteRefusesFormBody(t *testing.T) {
	api, _ := newAPITest(t)
	status, _ := api.call(http.MethodPost, apiPrefix+"session/login",
		map[string]string{"username": testPanelUser, "password": testPanelPassword},
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if status != http.StatusForbidden {
		t.Fatalf("form-encoded login = %d, want 403", status)
	}
}

func TestAPISetupFlow(t *testing.T) {
	saved := authFailDelay
	authFailDelay = 0
	t.Cleanup(func() { authFailDelay = saved })
	api, stub := newAPITest(t)
	stub.mu.Lock()
	stub.users = nil
	stub.bootstrapLive = true
	stub.mu.Unlock()
	if _, body := api.call(http.MethodGet, apiPrefix+"session", nil, nil); body["gate"] != "setup" {
		t.Fatalf("gate = %v, want setup", body["gate"])
	}
	status, body := api.call(http.MethodPost, apiPrefix+"session/setup",
		map[string]string{"bootstrap": "wrong", "username": "first", "password": "password123"}, nil)
	if status != http.StatusBadRequest || body["field"] != "bootstrap" {
		t.Fatalf("bad bootstrap = %d %v", status, body)
	}
	status, body = api.call(http.MethodPost, apiPrefix+"session/setup",
		map[string]string{"bootstrap": "stub-bootstrap", "username": "first", "password": "short"}, nil)
	if status != http.StatusBadRequest || body["field"] != "password" {
		t.Fatalf("weak password = %d %v", status, body)
	}
	status, body = api.call(http.MethodPost, apiPrefix+"session/setup",
		map[string]string{"bootstrap": "stub-bootstrap", "username": "first", "password": "password123"}, nil)
	if status != http.StatusOK || body["gate"] != "ready" || body["csrf"] == "" {
		t.Fatalf("setup = %d %v", status, body)
	}
}

func TestAPIUnknownEndpointIsJSON404(t *testing.T) {
	api, _ := newAPITest(t)
	status, body := api.call(http.MethodGet, apiPrefix+"nope", nil, nil)
	if status != http.StatusNotFound || body["code"] != "not_found" {
		t.Fatalf("unknown = %d %v", status, body)
	}
}

func TestWriteAgentFailurePassesStructuredRefusal(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAgentFailure(rec, &agentError{
		StatusCode: http.StatusBadRequest,
		Message:    `{"ok":false,"code":"product_root_unwritable","field":"root","message":"cannot create /srv/relkit/x"}`,
	})
	var body apiError
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusBadRequest || body.Code != "product_root_unwritable" || body.Field != "root" {
		t.Fatalf("structured = %d %+v", rec.Code, body)
	}
	rec = httptest.NewRecorder()
	writeAgentFailure(rec, &agentError{StatusCode: http.StatusForbidden, Message: "forbidden: only an admin"})
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusForbidden || body.Code != "forbidden" || body.Message != "forbidden: only an admin" {
		t.Fatalf("plain = %d %+v", rec.Code, body)
	}
}
