package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeConfigFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFileConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, ConfigName, `{
	  "addr": ":9000",
	  "dir": "/srv/releases",
	  "maxUpload": "512MiB",
	  "cache": {"noCache": ["index/"], "immutable": ["artifact/"], "defaultMaxAge": 30},
	  "gc": {"enabled": false, "interval": "15m"},
	  "publish": {"minProtocol": 2}
	}`)

	cfg, used, err := LoadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if used != path {
		t.Errorf("used = %q, want %q", used, path)
	}
	if cfg.Addr != ":9000" || cfg.Dir != "/srv/releases" || cfg.MaxUpload != "512MiB" {
		t.Errorf("unexpected config: %+v", cfg)
	}
	if cfg.Cache == nil || cfg.Cache.DefaultMaxAge == nil || *cfg.Cache.DefaultMaxAge != 30 {
		t.Errorf("cache not parsed: %+v", cfg.Cache)
	}
	if cfg.GC == nil || cfg.GC.Enabled == nil || *cfg.GC.Enabled || cfg.GC.Interval != "15m" {
		t.Errorf("gc not parsed: %+v", cfg.GC)
	}
	if cfg.Publish == nil || cfg.Publish.MinProtocol != 2 {
		t.Errorf("publish policy not parsed: %+v", cfg.Publish)
	}
}

func TestLoadFileConfigRejectsNegativePublishProtocol(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, ConfigName, `{"publish":{"minProtocol":-1}}`)
	if _, _, err := LoadFileConfig(path); err == nil ||
		!strings.Contains(err.Error(), "publish.minProtocol") {
		t.Fatalf("LoadFileConfig error = %v", err)
	}
}

func TestLoadFileConfigMissingIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	cfg, used, err := LoadFileConfig("")
	if err != nil {
		t.Fatalf("no config file should not be an error: %v", err)
	}
	if cfg != nil || used != "" {
		t.Errorf("got cfg=%v used=%q, want nil and empty", cfg, used)
	}
}

// A misspelled key that silently keeps the default is the worst kind of
// configuration bug: the server starts, reports success, and behaves
// differently than the file says.
func TestLoadFileConfigRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, ConfigName, `{"addr": ":9000", "nocache": ["index/"]}`)

	_, _, err := LoadFileConfig(path)
	if err == nil {
		t.Fatal("expected an error for the misspelled key 'nocache'")
	}
	if !strings.Contains(err.Error(), "nocache") {
		t.Errorf("error should name the offending field, got: %v", err)
	}
}

func TestLoadFileConfigFindsCwdFile(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, ConfigName, `{"addr": ":7777"}`)
	t.Chdir(dir)

	cfg, used, err := LoadFileConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || cfg.Addr != ":7777" {
		t.Fatalf("did not pick up %s from the working directory: %+v", ConfigName, cfg)
	}
	if used != ConfigName {
		t.Errorf("used = %q, want %q", used, ConfigName)
	}
}

func TestTokenFromConfigInline(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, ConfigName, `{"uploadToken": "s3cret"}`)

	cfg, _, err := LoadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := cfg.TokenFromFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != string(hashToken("s3cret")) {
		t.Error("inline token not hashed as expected")
	}
}

func TestTokenFromConfigFileIsRelativeToConfig(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, "tok", "from-file\n")
	path := writeConfigFile(t, dir, ConfigName, `{"uploadTokenFile": "tok"}`)

	cfg, _, err := LoadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := cfg.TokenFromFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != string(hashToken("from-file")) {
		t.Error("token file not resolved relative to the config file")
	}
}

func TestTokenFromConfigRejectsBothForms(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, ConfigName,
		`{"uploadToken": "a", "uploadTokenFile": "tok"}`)

	cfg, _, err := LoadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := cfg.TokenFromFileConfig(path); err == nil {
		t.Fatal("expected an error when both token forms are set")
	}
}

func TestTokenFromConfigEmptyMeansReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, ConfigName, `{"addr": ":8080"}`)

	cfg, _, err := LoadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := cfg.TokenFromFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if token != nil {
		t.Error("no token in config should leave uploads disabled")
	}
}

// Anyone who can read the token can publish, and publishing here means
// replacing binaries that clients will install and run.
func TestPermissionWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not reported on Windows")
	}
	dir := t.TempDir()
	path := writeConfigFile(t, dir, ConfigName, `{"uploadToken": "s3cret"}`)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := LoadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	_, warnings, err := cfg.TokenFromFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) == 0 {
		t.Fatal("a world-readable file holding a token should warn")
	}
	if !strings.Contains(warnings[0], "chmod 600") {
		t.Errorf("warning should say how to fix it, got: %s", warnings[0])
	}

	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, warnings, _ := cfg.TokenFromFileConfig(path); len(warnings) != 0 {
		t.Errorf("mode 0600 should not warn, got: %v", warnings)
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1024", 1024},
		{"512", 512},
		{"1KiB", 1 << 10},
		{"4GiB", 4 << 30},
		{"512MiB", 512 << 20},
		{"1.5GiB", 1610612736},
		{"100MB", 100 * 1000 * 1000},
		{" 8MiB ", 8 << 20},
		{"2g", 2 << 30},
	}
	for _, c := range cases {
		got, err := ParseSize(c.in)
		if err != nil {
			t.Errorf("ParseSize(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}

	for _, bad := range []string{"", "abc", "-1", "12XiB"} {
		if _, err := ParseSize(bad); err == nil {
			t.Errorf("ParseSize(%q) should fail", bad)
		}
	}
}

// The skeleton has to carry the RUP cache prefixes: a reader who does not yet
// know why they matter would otherwise leave them out, and the only symptom
// would be releases appearing minutes late.
func TestSkeletonIsUsableAndComplete(t *testing.T) {
	raw := Skeleton("/srv/releases")

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	var cfg FileConfig
	if err := dec.Decode(&cfg); err != nil {
		t.Fatalf("init writes a config it cannot itself load: %v", err)
	}

	if cfg.Dir != "/srv/releases" {
		t.Errorf("dir = %q", cfg.Dir)
	}
	if cfg.UploadTokenFile == "" {
		t.Error("skeleton should point at a token file")
	}
	if cfg.UploadToken != "" {
		t.Error("skeleton must not inline a token")
	}
	if cfg.Cache == nil {
		t.Fatal("skeleton must carry cache rules")
	}
	if len(cfg.Cache.NoCache) == 0 || cfg.Cache.NoCache[0] != "index/" {
		t.Errorf("noCache must contain index/, got %v", cfg.Cache.NoCache)
	}
	if len(cfg.Cache.Immutable) != 2 {
		t.Errorf("immutable should cover manifest/ and artifact/, got %v", cfg.Cache.Immutable)
	}
	if _, err := ParseSize(cfg.MaxUpload); err != nil {
		t.Errorf("skeleton maxUpload is not parseable: %v", err)
	}
	if _, err := ParseDuration(cfg.ShutdownTimeout); err != nil {
		t.Errorf("skeleton shutdownTimeout is not parseable: %v", err)
	}
	if cfg.GC == nil || cfg.GC.Enabled == nil || !*cfg.GC.Enabled {
		t.Errorf("skeleton must enable gc, got %+v", cfg.GC)
	}
	if _, err := ParseDuration(cfg.GC.Interval); err != nil {
		t.Errorf("skeleton gc.interval is not parseable: %v", err)
	}
}

func TestInitWritesTokenWithTightPermissions(t *testing.T) {
	dir := t.TempDir()
	if err := runInit(io.Discard, []string{"-dir", "/srv/releases", "-out", dir}); err != nil {
		t.Fatal(err)
	}

	tokenPath := filepath.Join(dir, "relkit-serve.token")
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("token mode = %04o, want 0600", info.Mode().Perm())
	}

	raw, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(string(raw))
	// 32 random bytes in base64url. Short enough to matter if it regresses:
	// this token is the only thing standing between the network and the
	// ability to replace binaries clients execute.
	if len(token) < 40 {
		t.Errorf("token is only %d chars: %q", len(token), token)
	}

	// The generated pair must load cleanly, or init has produced a deployment
	// that fails at first start.
	cfg, used, err := LoadFileConfig(filepath.Join(dir, ConfigName))
	if err != nil {
		t.Fatal(err)
	}
	loaded, _, err := cfg.TokenFromFileConfig(used)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded) != string(hashToken(token)) {
		t.Error("config does not resolve to the token init generated")
	}
}

func TestInitRefusesToClobber(t *testing.T) {
	dir := t.TempDir()
	if err := runInit(io.Discard, []string{"-out", dir}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "relkit-serve.token"))
	if err != nil {
		t.Fatal(err)
	}

	err = runInit(io.Discard, []string{"-out", dir})
	if err == nil {
		t.Fatal("re-running init must not silently replace a live token")
	}
	if !strings.Contains(err.Error(), "-force") {
		t.Errorf("error should mention -force, got: %v", err)
	}

	after, err := os.ReadFile(filepath.Join(dir, "relkit-serve.token"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("token changed despite the refusal")
	}

	if err := runInit(io.Discard, []string{"-out", dir, "-force"}); err != nil {
		t.Fatal(err)
	}
	forced, err := os.ReadFile(filepath.Join(dir, "relkit-serve.token"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) == string(forced) {
		t.Error("-force should have generated a new token")
	}
}

// Rotation must not revert hand-edited settings. A reverted cache prefix would
// show up only as releases arriving minutes late, long after anyone connects it
// to a token rotation.
func TestInitTokenOnlyLeavesConfigAlone(t *testing.T) {
	dir := t.TempDir()
	if err := runInit(io.Discard, []string{"-dir", "/srv/releases", "-out", dir}); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, ConfigName)
	edited := `{"addr": ":9999", "dir": "/srv/releases", ` +
		`"uploadTokenFile": "relkit-serve.token", ` +
		`"cache": {"noCache": ["index/"], "immutable": ["artifact/"]}}`
	if err := os.WriteFile(configPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	tokenPath := filepath.Join(dir, "relkit-serve.token")
	before, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}

	// No -force, and it must still succeed: rotation is a routine operation.
	if err := runInit(io.Discard, []string{"-out", dir, "-token-only"}); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) == string(after) {
		t.Error("token was not rotated")
	}

	current, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != edited {
		t.Errorf("config was rewritten:\n%s", current)
	}

	cfg, used, err := LoadFileConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded, _, err := cfg.TokenFromFileConfig(used)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded) != string(hashToken(strings.TrimSpace(string(after)))) {
		t.Error("edited config no longer resolves to the rotated token")
	}
}

func TestGeneratedTokensDiffer(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		token, err := generateToken()
		if err != nil {
			t.Fatal(err)
		}
		if seen[token] {
			t.Fatal("generateToken repeated a value")
		}
		seen[token] = true
	}
}

// The guide is embedded so that it cannot drift from the build being run, and
// so an agent on a fresh machine can read it without network or a checkout.
func TestEmbeddedGuideIsPresent(t *testing.T) {
	if len(agentGuide) < 2000 {
		t.Fatalf("embedded guide is only %d bytes; go:embed may have picked up a stub", len(agentGuide))
	}
	for _, needed := range []string{"## 0.", "## 3.", "noCache", "relkit-serve agent-guide"} {
		if !strings.Contains(agentGuide, needed) {
			t.Errorf("embedded guide is missing %q", needed)
		}
	}
}
