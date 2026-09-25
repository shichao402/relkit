package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
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
const version = "dev"

// ConfigName is looked up next to the binary and in /etc when -config is
// omitted. Which one was used is always logged at startup.
const ConfigName = "relkit-console.json"

var searchPaths = []string{ConfigName, "/etc/" + ConfigName}

// FileConfig mirrors the config file. Everything is optional; the defaults
// match what relkit-serve used for the panel.
type FileConfig struct {
	Addr          string      `json:"addr"`
	Dir           string      `json:"dir"`
	StateDir      string      `json:"stateDir,omitempty"`
	StatsFile     string      `json:"statsFile,omitempty"`
	AdminStateFile string     `json:"adminStateFile,omitempty"`
	Site          *SiteConfig `json:"site,omitempty"`
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
	var (
		configPath   = flag.String("config", "", "path to "+ConfigName)
		addr         = flag.String("addr", "127.0.0.1:8081", "address to listen on")
		dir          = flag.String("dir", ".", "release tree to read (read-only)")
		stateDir     = flag.String("state-dir", "", "agent state directory (site status)")
		showVersion  = flag.Bool("version", false, "print version and exit")
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
		adapter:    nil, // set below; failure to open is fatal
		stateDir:   *stateDir,
		site:       nil,
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

func statsPathOrDefault(dir string, cfg *FileConfig) string {
	if cfg != nil && cfg.StatsFile != "" {
		return resolveStatsPath(dir, cfg.StatsFile)
	}
	return defaultStatsPath(dir)
}
