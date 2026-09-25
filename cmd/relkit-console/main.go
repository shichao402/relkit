package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Command relkit-console is the operator panel binary split out of
// relkit-serve (ADR 0016). It renders the management plane: login/setup,
// the live portal, product pages, file tree and download stats.
//
// It serves no files and accepts no uploads. Every storage fact arrives
// through an Adapter (adapters.go); writes stay with relkit-store and
// relkit-agent. Pointing a browser at the console cannot publish anything.
//
// Usage:
//
//	relkit-console [-config PATH]              run the panel
//	relkit-console init [-dir DIR] [-out DIR]  write a panel config and a
//	                                           one-shot bootstrap token
//	relkit-console init -reset-admin           issue a new bootstrap; existing
//	                                           operators are wiped
//	relkit-console -version
//
// Config (relkit-console.json, next to the binary or /etc):
//
//	{
//	  "addr": "127.0.0.1:8081",
//	  "dir": "/srv/releases",         // release tree, read-only
//	  "stateDir": "/srv/relkit-agent-state", // agent state for site status
//	  "site": {...}                   // portal title/product blurbs (unchanged)
//	}
// Set by scripts/deploy/relkit.py build via -ldflags -X main.version=<stamp>.
var version = "dev"

// ConfigName is looked up next to the binary and in /etc when -config is
// omitted. Which one was used is always logged at startup.
const ConfigName = "relkit-console.json"

var searchPaths = []string{ConfigName, "/etc/" + ConfigName}

// FileConfig mirrors the config file. Everything is optional; the defaults
// match what relkit-serve used for the panel.
type FileConfig struct {
	Addr           string      `json:"addr"`
	Dir            string      `json:"dir"`
	StateDir       string      `json:"stateDir,omitempty"`
	StatsFile      string      `json:"statsFile,omitempty"`
	AdminStateFile string      `json:"adminStateFile,omitempty"`
	Site           *SiteConfig `json:"site,omitempty"`
}

func LoadFileConfig(explicit string) (*FileConfig, string, error) {
	path := explicit
	if path == "" {
		for _, candidate := range searchPaths {
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
		}
		if path == "" {
			return nil, "", nil
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}
	var cfg FileConfig
	if err := jsonDecodeStrict(raw, &cfg); err != nil {
		return nil, path, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, path, nil
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			if err := runInit(os.Stdout, os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}
	var (
		configPath  = flag.String("config", "", "path to "+ConfigName)
		addr        = flag.String("addr", "127.0.0.1:8081", "address to listen on")
		dir         = flag.String("dir", ".", "release tree to read (read-only)")
		stateDir    = flag.String("state-dir", "", "agent state directory (site status)")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("relkit-console %s\n", version)
		return
	}

	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	fileCfg, usedPath, err := LoadFileConfig(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if fileCfg != nil {
		if fileCfg.Addr != "" && !explicit["addr"] {
			*addr = fileCfg.Addr
		}
		if fileCfg.Dir != "" && !explicit["dir"] {
			*dir = fileCfg.Dir
		}
		if fileCfg.StateDir != "" && !explicit["state-dir"] {
			*stateDir = fileCfg.StateDir
		}
	}

	console := &console{
		adapter:  nil, // set below; failure to open is fatal
		stateDir: *stateDir,
		site:     nil,
	}
	if fileCfg != nil {
		console.site = fileCfg.Site
	}

	adapter, err := newRootAdapter("local", *dir)
	if err != nil {
		log.Fatalf("cannot read %s: %v", *dir, err)
	}
	console.adapter = adapter

	// Admin auth state follows the release tree unless configured otherwise;
	// same default layout relkit-serve used, so a box migrating to console
	// keeps its operator accounts.
	adminPath := resolveAdminPath(*dir, usedPath, adminStateFileFrom(fileCfg))
	admin, err := openAdminAuth(adminPath, *dir)
	if err != nil {
		log.Fatalf("admin: %v", err)
	}
	console.admin = admin
	console.stats = newDownloadStats(statsPathOrDefault(*dir, fileCfg), *dir)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           console.handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("relkit-console %s", version)
	if usedPath != "" {
		log.Printf("config: %s", usedPath)
	} else {
		log.Printf("config: none (flags and defaults only)")
	}
	log.Printf("panel on http://%s reading %s via %s adapter", *addr, *dir, adapter.Name())
	if *stateDir != "" {
		log.Printf("site status: %s", *stateDir)
	}
	log.Printf("panel: %s", admin.statusLog())

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}

// skeletonBytes renders the panel config a fresh box starts from. Only the
// paths matter; the site section is left for the operator to shape.
func skeletonBytes(dir string) []byte {
	cfg := &FileConfig{
		Addr: "127.0.0.1:8081",
		Dir:  dir,
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		panic(err) // FileConfig is plain strings; MarshalIndent cannot fail.
	}
	return append(raw, '\n')
}

// runInit writes a panel config skeleton plus a freshly minted one-shot
// bootstrap, so that a fresh console box needs no hand-written secrets.
//
// It is the admin half of serve's old init (ADR 0016): the storage half
// (config skeleton, upload tokens, product tokens) lives in
// `relkit-store init`, and a console box has no upload tokens of its own.
//
// Usage:
//
//	relkit-console init [-dir DIR] [-out DIR] [-force]
//	relkit-console init -reset-admin
func runInit(out io.Writer, args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	dir := fs.String("dir", "/srv/releases", "release tree the panel reads (read-only)")
	target := fs.String("out", ".", "where to write the config and bootstrap state")
	force := fs.Bool("force", false, "overwrite an existing config")
	resetAdmin := fs.Bool("reset-admin", false, "issue a new panel bootstrap; existing operators are wiped")
	fs.Parse(args)

	outDir := *target
	configPath := filepath.Join(outDir, ConfigName)

	// The reset path runs before MkdirAll: bringing /etc/relkit-console into
	// existence is the panel's job on a fresh box, not the break-glass path on
	// a broken one. -dir and -out stay valid: they locate the config, exactly
	// like serve's init did.
	if *resetAdmin {
		if *force {
			return fmt.Errorf("-reset-admin cannot be combined with -force")
		}
		return runInitResetAdmin(out, configPath, *dir)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	if _, err := os.Stat(configPath); err == nil && !*force {
		return fmt.Errorf("%s already exists; pass -force to overwrite "+
			"(this invalidates the current bootstrap)", configPath)
	}

	// Config first, bootstrap second. A crash between the two leaves a config
	// with no live bootstrap: the panel comes up locked rather than open.
	if err := os.WriteFile(configPath, skeletonBytes(*dir), 0o644); err != nil {
		return err
	}

	token, adminPath, adminRel, err := writeNewAdminState(*dir, outDir)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "config %s\n", configPath)
	if adminRel != "" {
		fmt.Fprintf(out, "NOTE: %s did not exist; admin state is next to the config.\n", *dir)
		fmt.Fprintf(out, "      Move it into the release directory before first start.\n")
	}
	printAdminBootstrap(out, adminPath, token)
	fmt.Fprintf(out, "\nPanel reads %s read-only; store serves the same tree.\n", *dir)
	fmt.Fprintf(out, "Then: relkit-console -config %s\n", configPath)
	fmt.Fprintf(out, "Storage side (upload tokens, product tokens): relkit-store init\n")
	return nil
}

// runInitResetAdmin wipes operator accounts and issues a new bootstrap. The
// running process keeps the old session key until it reloads, so restart
// after handing the new bootstrap to whoever will create the next account.
//
// This is the break-glass path the locked panel points at: it must work even
// when the config is gone, so it locates the state file through the -dir
// default rather than a config lookup.
func runInitResetAdmin(out io.Writer, configPath, dirFlag string) error {
	cfg, _, err := LoadFileConfig(configPath)
	if err != nil {
		return err
	}
	serveDir := dirFlag
	adminRel := ""
	if cfg != nil {
		serveDir = cfg.Dir
		if serveDir == "" {
			serveDir = dirFlag
		}
		adminRel = cfg.AdminStateFile
	}
	path := resolveAdminPath(serveDir, configPath, adminRel)
	token, doc, err := mintAdminDoc()
	if err != nil {
		return err
	}
	if err := writeAdminDoc(path, doc); err != nil {
		return err
	}
	fmt.Fprintf(out, "config %s\n", configPath)
	printAdminBootstrap(out, path, token)
	fmt.Fprintf(out, "\nExisting operator accounts and sessions are gone. Restart to load it:\n")
	fmt.Fprintf(out, "  systemctl restart relkit-console\n")
	return nil
}

func statsPathOrDefault(dir string, cfg *FileConfig) string {
	if cfg != nil && cfg.StatsFile != "" {
		return resolveStatsPath(dir, cfg.StatsFile)
	}
	return defaultStatsPath(dir)
}
