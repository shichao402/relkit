// Command relkit-agent receives staged trees from CI and publishes them
// (signing keys and COS credentials stay on the host).
//
//	relkit-agent [flags]                         run the server
//	relkit-agent init -list-products             list products in the config
//	relkit-agent init -product <id> [-root path] add a product (shared token)
//	relkit-agent init -product <id> -remove      drop a product from the map
//	relkit-agent -version
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var version = "0.1.2"

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
	mux := http.NewServeMux()
	mux.HandleFunc("/-/health", srv.handleHealth)
	mux.HandleFunc("/v1/drop/", srv.handleDrop)
	mux.HandleFunc("/v1/staged/", srv.handleStaged)
	mux.HandleFunc("/v1/publish", srv.handlePublish)

	log.Printf("relkit-agent %s listening on %s", version, cfg.Addr)
	if cfg.uploadTokenHash == nil {
		log.Printf("WARNING: no upload token configured; write endpoints return 405")
	}
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Minute,
		WriteTimeout:      30 * time.Minute,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("serve: %v", err)
		return 1
	}
	return 0
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
