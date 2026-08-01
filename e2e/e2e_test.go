package e2e_test

import (
	"crypto/hmac"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/chain"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/jsonio"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/testutil"
)

func TestCLIEndToEndLocalBackend(t *testing.T) {
	exe := buildRelkit(t)
	project := t.TempDir()
	dist := filepath.Join(project, "dist")
	mustMkdirAll(t, dist)

	backendsOut := runRelkit(t, exe, project, nil, 0, "backends")
	assertContains(t, backendsOut, "local")
	assertContains(t, backendsOut, "static-http")
	assertContains(t, backendsOut, "http-put")

	guideOut := runRelkit(t, exe, project, nil, 0, "agent-guide")
	assertContains(t, guideOut, "RUP 发布操作手册")

	runRelkit(t, exe, project, nil, 0, "init", "--product", "demoapp")
	runRelkit(t, exe, project, nil, 0, "keygen", "--key-id", "k1", "--out", "keys", "--update-config")
	setPrivateKeyPath(t, project, "keys/k1.private.pb")

	writeArtifact(t, dist, "demoapp-1.0.0-win-x64.zip", "win 1.0.0 ", 64)
	writeArtifact(t, dist, "demoapp-1.0.0-mac-arm64.zip", "mac 1.0.0 ", 64)
	stageOut := runRelkit(t, exe, project, nil, 0,
		"stage", "1.0.0", "--code", "100",
		"--add", filepath.Join(dist, "demoapp-1.0.0-win-x64.zip"), "os=windows,arch=x64",
		"--add", filepath.Join(dist, "demoapp-1.0.0-mac-arm64.zip"), "os=macos,arch=arm64",
	)
	assertContains(t, stageOut, "windows-x64")
	assertContains(t, stageOut, "kind inferred as archive")

	dryRun := runRelkit(t, exe, project, nil, 0, "publish", "1.0.0", "--dry-run")
	assertContains(t, dryRun, "nothing uploaded")
	assertContains(t, dryRun, "pointer, written last")
	if _, err := os.Stat(filepath.Join(project, "dist", "publish")); !os.IsNotExist(err) {
		t.Fatalf("dry run unexpectedly created dist/publish")
	}

	runRelkit(t, exe, project, nil, 0, "publish", "1.0.0")
	index := loadPublishedIndex(t, project, "demoapp", "stable")
	if index.Sequence != 1 {
		t.Fatalf("sequence mismatch: got %d want 1", index.Sequence)
	}
	if len(index.Versions) != 1 {
		t.Fatalf("versions mismatch: got %d want 1", len(index.Versions))
	}

	inspectOut := runRelkit(t, exe, project, nil, 0, "inspect", "--file", filepath.Join(project, "dist", "publish", "index", "demoapp", "stable.pb"))
	assertContains(t, inspectOut, "\"versions\"")

	writeArtifact(t, dist, "demoapp-1.1.0-win-x64.zip", "win 1.1.0 ", 64)
	runRelkit(t, exe, project, nil, 0,
		"stage", "1.1.0", "--code", "110",
		"--add", filepath.Join(dist, "demoapp-1.1.0-win-x64.zip"), "os=windows,arch=x64",
	)
	runRelkit(t, exe, project, nil, 0, "publish", "1.1.0")

	writeArtifact(t, dist, "demoapp-1.5.0-win-x64.zip", "win 1.5.0 ", 64)
	writeArtifact(t, dist, "demoapp-1.5.0-mac-arm64.zip", "mac 1.5.0 ", 64)
	runRelkit(t, exe, project, nil, 0,
		"stage", "1.5.0", "--code", "150", "--min-from", "110", "--notes", "config format changed",
		"--add", filepath.Join(dist, "demoapp-1.5.0-win-x64.zip"), "os=windows,arch=x64",
		"--add", filepath.Join(dist, "demoapp-1.5.0-mac-arm64.zip"), "os=macos,arch=arm64",
	)
	simulated := runRelkit(t, exe, project, nil, 0, "simulate", "--with-staged", "1.5.0", "--from", "all")
	assertContains(t, simulated, "1.5.0")
	if strings.Contains(simulated, "STRANDED") {
		t.Fatalf("simulate unexpectedly reported stranded codes:\n%s", simulated)
	}
	runRelkit(t, exe, project, nil, 0, "publish", "1.5.0")

	index = loadPublishedIndex(t, project, "demoapp", "stable")
	if index.Sequence != 3 {
		t.Fatalf("sequence mismatch: got %d want 3", index.Sequence)
	}
	pathFromZero := versionsOf(chain.ResolveUpgradePath(index, 0))
	assertStringSlices(t, pathFromZero, []string{"1.1.0", "1.5.0"})
	pathFrom110 := versionsOf(chain.ResolveUpgradePath(index, 110))
	assertStringSlices(t, pathFrom110, []string{"1.5.0"})
	if unreachable := chain.UnreachableStartCodes(index); len(unreachable) != 0 {
		t.Fatalf("unexpected unreachable codes: %v", unreachable)
	}

	verifyOut := runRelkit(t, exe, project, nil, 0, "verify", "--deep")
	assertContains(t, verifyOut, "verify passed")

	writeArtifact(t, dist, "demoapp-2.0.0-win-x64.zip", "win 2.0.0 ", 64)
	runRelkit(t, exe, project, nil, 0,
		"stage", "2.0.0", "--code", "200", "--min-from", "190",
		"--add", filepath.Join(dist, "demoapp-2.0.0-win-x64.zip"), "os=windows,arch=x64",
	)
	stranding := runRelkit(t, exe, project, nil, 2, "publish", "2.0.0", "--dry-run")
	assertContains(t, stranding, "unreachable")
	assertContains(t, stranding, "0, 100, 110, 150")
	index = loadPublishedIndex(t, project, "demoapp", "stable")
	if index.Sequence != 3 {
		t.Fatalf("pointer changed after rejected publish: sequence=%d", index.Sequence)
	}
	if _, err := os.Stat(filepath.Join(project, "dist", "publish", "artifact", "demoapp", "2.0.0")); !os.IsNotExist(err) {
		t.Fatalf("rejected publish unexpectedly uploaded artifact")
	}

	writeArtifact(t, dist, "demoapp-0.9.0-win-x64.zip", "win 0.9.0 ", 64)
	runRelkit(t, exe, project, nil, 0,
		"stage", "0.9.0", "--code", "90",
		"--add", filepath.Join(dist, "demoapp-0.9.0-win-x64.zip"), "os=windows,arch=x64",
	)
	rollback := runRelkit(t, exe, project, nil, 2, "publish", "0.9.0", "--dry-run")
	assertContains(t, rollback, "not greater than the highest existing code")
	assertContains(t, rollback, "--allow-backfill")

	stagedArtifact := filepath.Join(project, ".relkit", "staged", "1.5.0", "artifacts", "demoapp-1.5.0-win-x64.zip")
	file, err := os.OpenFile(stagedArtifact, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("tampered")); err != nil {
		t.Fatal(err)
	}
	file.Close()

	tampered := runRelkit(t, exe, project, nil, 2, "publish", "1.5.0", "--dry-run")
	assertContains(t, tampered, "staging tree no longer matches staged.pb")
	verifyAfterTamper := runRelkit(t, exe, project, nil, 0, "verify")
	assertContains(t, verifyAfterTamper, "verify passed")
}

func TestStaticHTTPBackend(t *testing.T) {
	exe := buildRelkit(t)
	project := t.TempDir()
	dist := filepath.Join(project, "dist")
	www := filepath.Join(project, "www")
	mustMkdirAll(t, dist)
	mustMkdirAll(t, www)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/r/") {
			http.Redirect(w, r, strings.TrimPrefix(r.URL.Path, "/r"), http.StatusFound)
			return
		}
		http.FileServer(http.Dir(www)).ServeHTTP(w, r)
	}))
	defer server.Close()

	runRelkit(t, exe, project, nil, 0, "init", "--product", "siteapp")
	runRelkit(t, exe, project, nil, 0, "keygen", "--key-id", "k1", "--out", "keys", "--update-config")
	setPrivateKeyPath(t, project, "keys/k1.private.pb")
	addBackend(t, project, "site", map[string]any{
		"type":     "static-http",
		"baseUrl":  server.URL + "/r/",
		"stageDir": "www",
	})

	writeArtifact(t, dist, "siteapp-1.0.0-win-x64.zip", "site 1.0.0 ", 64)
	runRelkit(t, exe, project, nil, 0,
		"stage", "1.0.0", "--code", "100",
		"--add", filepath.Join(dist, "siteapp-1.0.0-win-x64.zip"), "os=windows,arch=x64",
	)
	runRelkit(t, exe, project, nil, 0, "publish", "1.0.0", "--to", "site")
	out := runRelkit(t, exe, project, nil, 0, "verify", "--to", "site", "--deep")
	assertContains(t, out, "verify passed")
	assertContains(t, out, "HEAD")

	addBackend(t, project, "audit", map[string]any{
		"type":    "static-http",
		"baseUrl": server.URL + "/r/",
	})
	out = runRelkit(t, exe, project, nil, 0, "verify", "--to", "audit", "--deep")
	assertContains(t, out, "verify passed")
}

func TestHTTPPutBackend(t *testing.T) {
	exe := buildRelkit(t)
	project := t.TempDir()
	dist := filepath.Join(project, "dist")
	store := filepath.Join(project, "store")
	mustMkdirAll(t, dist)
	mustMkdirAll(t, store)

	token := "e2e-upload-token"
	server := httptest.NewServer(newUploadHandler(t, store, token))
	defer server.Close()

	runRelkit(t, exe, project, nil, 0, "init", "--product", "servedapp")
	runRelkit(t, exe, project, nil, 0, "keygen", "--key-id", "srv", "--out", "keys", "--update-config")
	setPrivateKeyPath(t, project, "keys/srv.private.pb")
	addBackend(t, project, "dl", map[string]any{
		"type":     "http-put",
		"baseUrl":  server.URL + "/",
		"tokenEnv": "RELKIT_UPLOAD_TOKEN",
	})
	setPublishTo(t, project, []string{"dl"})

	env := map[string]string{"RELKIT_UPLOAD_TOKEN": token}
	writeArtifact(t, dist, "servedapp-1.0.0-win-x64.zip", "served 1.0.0 ", 30000)
	runRelkit(t, exe, project, nil, 0,
		"stage", "1.0.0", "--code", "100",
		"--add", filepath.Join(dist, "servedapp-1.0.0-win-x64.zip"), "os=windows,arch=x64",
	)
	runRelkit(t, exe, project, env, 0, "publish", "1.0.0")
	out := runRelkit(t, exe, project, env, 0, "verify", "--deep")
	assertContains(t, out, "verify passed")

	writeArtifact(t, dist, "servedapp-1.1.0-win-x64.zip", "served 1.1.0 ", 30000)
	runRelkit(t, exe, project, nil, 0,
		"stage", "1.1.0", "--code", "110",
		"--add", filepath.Join(dist, "servedapp-1.1.0-win-x64.zip"), "os=windows,arch=x64",
	)
	runRelkit(t, exe, project, env, 0, "publish", "1.1.0")
	out = runRelkit(t, exe, project, env, 0, "verify", "--deep")
	assertContains(t, out, "verify passed")

	writeArtifact(t, dist, "servedapp-1.2.0-win-x64.zip", "served 1.2.0 ", 64)
	runRelkit(t, exe, project, nil, 0,
		"stage", "1.2.0", "--code", "120",
		"--add", filepath.Join(dist, "servedapp-1.2.0-win-x64.zip"), "os=windows,arch=x64",
	)
	missingToken := runRelkit(t, exe, project, nil, 2, "publish", "1.2.0")
	assertContains(t, missingToken, "RELKIT_UPLOAD_TOKEN")
}

func buildRelkit(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "relkit.exe")
	cmd := exec.Command("go", "build", "-o", exe, "./cmd/relkit")
	cmd.Dir = testutil.RepoRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, output)
	}
	return exe
}

func runRelkit(t *testing.T, exe string, cwd string, extraEnv map[string]string, expectCode int, args ...string) string {
	t.Helper()
	cmd := exec.Command(exe, args...)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "GODEBUG=http2client=0")
	for key, value := range extraEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("command failed to start: %v", err)
		}
	}
	if exitCode != expectCode {
		t.Fatalf("relkit %s exit mismatch: got %d want %d\n%s", strings.Join(args, " "), exitCode, expectCode, output)
	}
	return string(output)
}

func setPrivateKeyPath(t *testing.T, project string, value string) {
	t.Helper()
	updateConfigJSON(t, filepath.Join(project, "relkit.json"), func(doc map[string]any) {
		signing := doc["signing"].(map[string]any)
		signing["privateKeyPath"] = value
	})
}

func addBackend(t *testing.T, project string, name string, entry map[string]any) {
	t.Helper()
	updateConfigJSON(t, filepath.Join(project, "relkit.json"), func(doc map[string]any) {
		backendsMap := doc["backends"].(map[string]any)
		backendsMap[name] = entry
	})
}

func setPublishTo(t *testing.T, project string, backends []string) {
	t.Helper()
	updateConfigJSON(t, filepath.Join(project, "relkit.json"), func(doc map[string]any) {
		values := make([]any, 0, len(backends))
		for _, backend := range backends {
			values = append(values, backend)
		}
		doc["publishTo"] = values
	})
}

func updateConfigJSON(t *testing.T, path string, mutate func(map[string]any)) {
	t.Helper()
	var doc map[string]any
	if err := jsonio.LoadPathLenient(path, &doc); err != nil {
		t.Fatal(err)
	}
	mutate(doc)
	data, err := jsonio.MarshalPretty(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonio.WritePath(path, data); err != nil {
		t.Fatal(err)
	}
}

func writeArtifact(t *testing.T, dir string, name string, payload string, repeat int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	data := strings.Repeat(payload, repeat)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadPublishedIndex(t *testing.T, project string, product string, channel string) *model.IndexDocument {
	t.Helper()
	cfg, err := config.Load(filepath.Join(project, "relkit.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, "dist", "publish", "index", product, channel+".pb")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	env, err := rupv2.UnmarshalEnvelope(data)
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := cfg.TrustedPublicKeys()
	if err != nil {
		t.Fatal(err)
	}
	index, err := envelope.OpenEnvelope(env, trusted)
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func versionsOf(path []model.VersionNode) []string {
	versions := make([]string, 0, len(path))
	for _, node := range path {
		versions = append(versions, node.Version)
	}
	return versions
}

func assertContains(t *testing.T, text string, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("expected output to contain %q\n%s", want, text)
	}
}

func assertStringSlices(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slice length mismatch: got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("slice mismatch: got %v want %v", got, want)
		}
	}
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

type uploadHandler struct {
	t     *testing.T
	dir   string
	token string
}

func newUploadHandler(t *testing.T, dir string, token string) http.Handler {
	return &uploadHandler{t: t, dir: dir, token: token}
}

func (h *uploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		h.handlePut(w, r)
	case http.MethodGet, http.MethodHead:
		http.FileServer(http.Dir(h.dir)).ServeHTTP(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *uploadHandler) handlePut(w http.ResponseWriter, r *http.Request) {
	expected := "Bearer " + h.token
	if !hmac.Equal([]byte(r.Header.Get("Authorization")), []byte(expected)) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/")
	if key == "" || strings.HasSuffix(key, "/") || strings.Contains(key, "..") {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	target := filepath.Join(append([]string{h.dir}, strings.Split(key, "/")...)...)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		h.t.Fatalf("mkdir failed: %v", err)
	}
	file, err := os.Create(target)
	if err != nil {
		h.t.Fatalf("create failed: %v", err)
	}
	defer file.Close()
	if _, err := io.Copy(file, r.Body); err != nil {
		h.t.Fatalf("copy failed: %v", err)
	}
	w.WriteHeader(http.StatusCreated)
}
