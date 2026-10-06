//	relkit-agent [flags]                         run the server
//	relkit-agent init …                          internal: called by product relkit_host.py
//	relkit-agent site-rebuild [-config PATH]     rebuild the static release site
//	relkit-agent unpublish [flags]               remove one published version
//	relkit-agent -version
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
	if len(cfg.credentials) == 0 {
		log.Printf("WARNING: no product upload tokens configured; write endpoints return 405")
	} else {
		log.Printf("product upload tokens: %d", len(cfg.credentials))
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
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: config: %v\n", err)
		return 1
	}
	srv := NewServer(cfg)
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
	dryRun := fs.Bool("dry-run", false, "validate only, remove nothing")
	var to []string
	fs.Func("to", "target backend (repeatable)", func(value string) error { to = append(to, value); return nil })
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if *product == "" || *version == "" {
		fmt.Fprintln(os.Stderr, "usage: relkit-agent unpublish -product <id> -version <version> [-config PATH] [-addr HOST:PORT] [-to name] [--dry-run]")
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

	token, err := productToken(cfg, *product)
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

// productToken reads the plaintext token for product from the agent config's
// uploadTokens entries. The CLI runs on the same host as the agent, so the
// token files are readable with the same privileges.
func productToken(cfg *Config, product string) (string, error) {
	for _, cred := range cfg.credentials {
		for _, id := range cred.products {
			if id == product {
				return credTokenFromFile(cfg, product)
			}
		}
	}
	return "", fmt.Errorf("product %q has no upload token entry in the agent config", product)
}

func credTokenFromFile(cfg *Config, product string) (string, error) {
	// The token files are listed in the raw config; reload it to find the path.
	raw, err := loadFileConfig(cfg.ConfigPath)
	if err != nil {
		return "", err
	}
	for _, entry := range raw.UploadTokens {
		for _, id := range entry.Products {
			if id != product {
				continue
			}
			path := entry.File
			if !filepath.IsAbs(path) {
				path = filepath.Join(filepath.Dir(cfg.ConfigPath), path)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(data)), nil
		}
	}
	return "", fmt.Errorf("no token file for %s", product)
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
