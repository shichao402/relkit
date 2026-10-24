// relkit-agent [flags]                         run the server
// relkit-agent init -bootstrap                 mint the one-shot first-operator credential
// relkit-agent site-rebuild [-config PATH]     rebuild the static release site
// relkit-agent unpublish [flags]               remove one published version
// relkit-agent -version
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/publishproto"
)

var version = "0.1.5"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	if len(argv) > 0 && argv[0] == "init" {
		if err := runInit(os.Stdout, argv[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		return 0
	}
	if len(argv) > 0 && argv[0] == "site-rebuild" {
		return runSiteRebuild(argv[1:])
	}
	if len(argv) > 0 && argv[0] == "unpublish" {
		return runUnpublish(argv[1:])
	}

	fs := flag.NewFlagSet("relkit-agent", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "relkit-agent.json", "agent config path")
	addr := fs.String("addr", "127.0.0.1:8787", "listen address")
	showVersion := fs.Bool("version", false, "print version")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println("relkit-agent " + version)
		return 0
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	if *addr != "" {
		cfg.Addr = *addr
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8787"
	}

	srv := NewServer(cfg)
	log.Printf("relkit-agent %s listening on %s (maxUpload %d partSize %d maxPartConcurrency %d)", version, cfg.Addr, cfg.MaxUpload, cfg.PartSize, cfg.MaxPartConcurrency)
	if cfg.admin != nil {
		log.Printf("admin state %s", cfg.admin.Path())
		if cfg.admin.BootstrapLive() {
			log.Printf("WARNING: no operator account yet; run `relkit-agent init -bootstrap` to create the first one")
		}
	}
	log.Printf("products: %d, live product tokens: %d", len(cfg.Products), len(cfg.credentials))
	if len(cfg.credentials) == 0 {
		log.Printf("WARNING: no product token has been issued; write endpoints return 405")
	}
	server := newHTTPServer(cfg, logRequests(srv.Handler()))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("serve: %v", err)
		return 1
	}
	return 0
}

func runSiteRebuild(argv []string) int {
	fs := flag.NewFlagSet("relkit-agent site-rebuild", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "relkit-agent.json", "agent config path")
	// Emptying the shelf is an explicit operation. Without it the complete
	// snapshot guard refuses to write a catalog with no products, which would
	// leave the human page advertising versions that no longer exist.
	allowEmpty := fs.Bool("allow-empty", false, "publish a snapshot with no products at all (after retiring every product)")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: config: %v\n", err)
		return 1
	}
	srv := NewServer(cfg)
	srv.allowEmptySite = *allowEmpty
	changed, err := srv.rebuildSite(func(line string) { fmt.Fprintln(os.Stdout, line) })
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if changed {
		fmt.Fprintln(os.Stdout, "site rebuild: deployed")
	}
	return 0
}

// runUnpublish removes one published version by calling the running agent's
// /v1/unpublish endpoint. Going through the agent (instead of hitting the
// store directly) keeps the product lock shared with concurrent publishes.
func runUnpublish(argv []string) int {
	fs := flag.NewFlagSet("relkit-agent unpublish", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "relkit-agent.json", "agent config path")
	addr := fs.String("addr", "", "agent address (defaults to the configured addr)")
	product := fs.String("product", "", "product id")
	version := fs.String("version", "", "version to remove")
	tokenFlag := fs.String("token", "", "product token (defaults to the RELKIT_UPLOAD_TOKEN environment variable)")
	dryRun := fs.Bool("dry-run", false, "validate only, remove nothing")
	var to []string
	fs.Func("to", "target backend (repeatable)", func(value string) error { to = append(to, value); return nil })
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if *product == "" || *version == "" {
		fmt.Fprintln(os.Stderr, "usage: relkit-agent unpublish -product <id> -version <version> [-config PATH] [-addr HOST:PORT] [-to name] [--token TOKEN] [--dry-run]")
		return 2
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: config: %v\n", err)
		return 1
	}
	target := *addr
	if target == "" {
		target = cfg.Addr
	}
	if target == "" {
		target = "127.0.0.1:8787"
	}

	token, err := productToken(*tokenFlag, *product)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	body, _ := json.Marshal(map[string]any{
		"product": *product,
		"version": *version,
		"to":      to,
		"dryRun":  *dryRun,
	})
	req, err := http.NewRequest(http.MethodPost, "http://"+target+"/v1/unpublish", strings.NewReader(string(body)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	publishproto.Apply(req.Header)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: agent at %s: %v\n", target, err)
		return 1
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "agent returned %d: %s\n", resp.StatusCode, firstLine(raw))
		return 1
	}
	var out struct {
		OK       bool     `json:"ok"`
		Sequence int64    `json:"sequence"`
		Channel  string   `json:"channel"`
		Log      []string `json:"log"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	for _, line := range out.Log {
		fmt.Println(line)
	}
	return 0
}

// productToken resolves the plaintext token for a CLI call. The server keeps
// only hashes (requirement 11.3), so there is no file to read back: the caller
// supplies the value, either with -token or through the same environment
// variable a host CI uses.
func productToken(flagValue, product string) (string, error) {
	token := strings.TrimSpace(flagValue)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("RELKIT_UPLOAD_TOKEN"))
	}
	if token == "" {
		return "", fmt.Errorf("no token for %q: pass -token or set RELKIT_UPLOAD_TOKEN", product)
	}
	return token, nil
}

func firstLine(data []byte) string {
	text := strings.TrimSpace(string(data))
	if idx := strings.Index(text, "\n"); idx >= 0 {
		text = text[:idx]
	}
	return text
}

// newHTTPServer deliberately leaves ReadTimeout and WriteTimeout unset. A
// staged upload is bounded by maxUpload (GiB scale), and publish signs and
// pushes those bytes to the backend before it answers, so any fixed deadline
// eventually cuts off a legitimate release from a slow link. The real bounds
// are ReadHeaderTimeout plus the MaxBytesReader in each handler.
func newHTTPServer(cfg *Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(rw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.code, time.Since(start).Round(time.Millisecond))
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func cleanVersion(version string) (string, bool) {
	version = strings.TrimSpace(version)
	if version == "" || strings.Contains(version, "..") || strings.ContainsAny(version, `/\`) {
		return "", false
	}
	return version, true
}

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
