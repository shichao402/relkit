package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/directory"
	"github.com/shichao402/relkit/internal/publish"
	"github.com/shichao402/relkit/internal/stage"
)

type ProductConfig struct {
	Root string `json:"root"`
}

type FileConfig struct {
	Addr            string                    `json:"addr"`
	UploadToken     string                    `json:"uploadToken,omitempty"`
	UploadTokenFile string                    `json:"uploadTokenFile,omitempty"`
	MaxUpload       string                    `json:"maxUpload,omitempty"`
	MaxFiles        int                       `json:"maxFiles,omitempty"`
	StateDir        string                    `json:"stateDir,omitempty"`
	Products        map[string]ProductConfig  `json:"products"`
}

type Config struct {
	Addr            string
	uploadTokenHash []byte
	MaxUpload       int64
	MaxFiles        int
	StateDir        string
	Products        map[string]ProductConfig
	ConfigPath      string
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw FileConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if len(raw.Products) == 0 {
		return nil, fmt.Errorf("products map is required")
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
	token, err := loadToken(raw)
	if err != nil {
		return nil, err
	}
	if token != "" {
		cfg.uploadTokenHash = hashToken(token)
	}
	for name, p := range cfg.Products {
		if strings.TrimSpace(p.Root) == "" {
			return nil, fmt.Errorf("product %q: root is required", name)
		}
		root := p.Root
		if !filepath.IsAbs(root) {
			root = filepath.Join(filepath.Dir(path), root)
		}
		p.Root = mustAbs(root)
		cfg.Products[name] = p
	}
	return cfg, nil
}

func loadToken(raw FileConfig) (string, error) {
	if raw.UploadToken != "" && raw.UploadTokenFile != "" {
		return "", fmt.Errorf("set only one of uploadToken / uploadTokenFile")
	}
	if raw.UploadToken != "" {
		return strings.TrimSpace(raw.UploadToken), nil
	}
	if raw.UploadTokenFile != "" {
		data, err := os.ReadFile(raw.UploadTokenFile)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}
	if env := strings.TrimSpace(os.Getenv("RELKIT_AGENT_TOKEN")); env != "" {
		return env, nil
	}
	return "", nil
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

func (s *Server) authorized(r *http.Request) bool {
	if s.cfg.uploadTokenHash == nil {
		return false
	}
	header := r.Header.Get("Authorization")
	value, found := strings.CutPrefix(header, "Bearer ")
	if !found {
		return false
	}
	presented := hashToken(strings.TrimSpace(value))
	return subtle.ConstantTimeCompare(presented, s.cfg.uploadTokenHash) == 1
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.uploadTokenHash == nil {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "uploads are disabled on this server", http.StatusMethodNotAllowed)
		return false
	}
	if !s.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-agent"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
	if !s.requireAuth(w, r) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/staged/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "expected /v1/staged/{product}/{version}", http.StatusBadRequest)
		return
	}
	product, versionRaw := parts[0], parts[1]
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

	dest := stage.StagingDir(pc.Root, version)
	_ = os.RemoveAll(dest)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		http.Error(w, "mkdir staged", http.StatusInternalServerError)
		return
	}
	if err := extractTarGz(tmpPath, dest, s.cfg.MaxFiles); err != nil {
		_ = os.RemoveAll(dest)
		http.Error(w, "extract: "+err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := stage.LoadStaged(pc.Root, version); err != nil {
		_ = os.RemoveAll(dest)
		http.Error(w, "invalid staged tree: "+err.Error(), http.StatusBadRequest)
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

type publishRequest struct {
	Product       string   `json:"product"`
	Version       string   `json:"version"`
	To            []string `json:"to"`
	DryRun        bool     `json:"dryRun"`
	AllowBackfill bool     `json:"allowBackfill"`
	AllowPartial  bool     `json:"allowPartial"`
	IdempotencyKey string  `json:"idempotencyKey"`
	StagedSHA256  string   `json:"stagedSha256"`
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAuth(w, r) {
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
	pc, ok := s.cfg.Products[req.Product]
	if !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return
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

	cfgPath := filepath.Join(pc.Root, config.ConfigName)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		http.Error(w, "load relkit.json: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if cfg.Product != "" && cfg.Product != req.Product {
		http.Error(w, "product mismatch with relkit.json", http.StatusBadRequest)
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
