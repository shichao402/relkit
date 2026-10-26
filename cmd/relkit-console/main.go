package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// Command relkit-console is the operator panel binary split out of
// relkit-serve (ADR 0016). It serves the management plane: the JSON API behind
// the single-page front end, the app shell, the fixed download URLs and the
// health endpoint.
//
// It serves no uploads and publishes nothing. Every storage fact arrives over
// HTTP from relkit-store through the store adapter (adapters.go), whether or
// not the store runs on the same box; writes are forwarded to relkit-agent
// under the operator's own bearer. Pointing a browser at the console cannot
// publish anything.
//
// Usage:
//
//	relkit-console [-config PATH]                     run the panel
//	relkit-console init -store URL [-out DIR]         write a panel config and
//	                                                  the panel's cookie key
//	relkit-console init -reset-admin                  issue a new cookie key;
//	                                                  every session signs out
//	relkit-console -version
//
// Config (relkit-console.json, next to the binary or /etc):
//
//	{
//	  "addr": "127.0.0.1:8081",
//	  "store": "http://127.0.0.1:8080",          // relkit-store, read over HTTP
//	  "agent": "http://127.0.0.1:8787",
//	  "adminStateFile": "/var/lib/relkit-console/panel.json",
//	  "site": {...}             // product blurbs for products without their own
//	}
//
// Set by scripts/deploy/relkit.py build via -ldflags -X main.version=<stamp>.
var version = "dev"

// ConfigName is looked up next to the binary and in /etc when -config is
// omitted. Which one was used is always logged at startup.
const ConfigName = "relkit-console.json"

// defaultPanelStatePath is where the cookie key lives when the config does not
// say. It is the unit's StateDirectory, outside the release tree: the panel
// owns this file, the store must never see it.
const defaultPanelStatePath = "/var/lib/relkit-console/panel.json"

var searchPaths = []string{ConfigName, "/etc/" + ConfigName}

// FileConfig mirrors the config file. Decoding is strict: a key this binary
// does not know is a startup error, not a silently ignored setting.
type FileConfig struct {
	Addr           string      `json:"addr"`
	Store          string      `json:"store"`
	Agent          string      `json:"agent,omitempty"`
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
		store       = flag.String("store", "", "relkit-store base URL the panel reads (required)")
		agentAddr   = flag.String("agent", "", "agent server base URL this panel manages (requirement 13.6)")
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
		if fileCfg.Store != "" && !explicit["store"] {
			*store = fileCfg.Store
		}
		if fileCfg.Agent != "" && !explicit["agent"] {
			*agentAddr = fileCfg.Agent
		}
	}
	if *store == "" {
		log.Fatalf("no relkit-store configured: set \"store\" in %s or pass -store", ConfigName)
	}

	adapter, err := newStoreAdapter("store:"+hostOf(*store), *store)
	if err != nil {
		log.Fatalf("cannot open data source: %v", err)
	}
	console := &console{adapter: adapter}
	if fileCfg != nil {
		console.site = fileCfg.Site
	}

	// Panel state is one cookie signing key; the authority for users, roles,
	// products and tokens lives on the agent (requirement 13.6). A box with no
	// agent configured still serves the read-only pages, it just cannot render
	// the management screens.
	adminPath, err := resolveAdminPath(adminStateFileFrom(fileCfg))
	if err != nil {
		log.Fatalf("admin: %v", err)
	}
	admin, err := openPanelSession(adminPath, *agentAddr)
	if err != nil {
		log.Fatalf("admin: %v", err)
	}
	console.admin = admin

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
	log.Printf("panel on http://%s reading %s via %s adapter", *addr, *store, adapter.Name())
	log.Printf("panel state: %s", adminPath)
	log.Printf("panel: %s", admin.statusLog())

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}

// skeletonBytes renders the panel config a fresh box starts from. Only the
// endpoints matter; the site section is left for the operator to shape.
func skeletonBytes(store, agent, statePath string) []byte {
	cfg := &FileConfig{
		Addr:           "127.0.0.1:8081",
		Store:          store,
		Agent:          agent,
		AdminStateFile: statePath,
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		panic(err) // FileConfig is plain strings; MarshalIndent cannot fail.
	}
	return append(raw, '\n')
}

// runInit writes a panel config skeleton plus a freshly minted cookie key, so
// that a fresh console box needs no hand-written secrets.
//
// It is the admin half of serve's old init (ADR 0016): the storage half lives
// in `relkit-store init`, and a console box has no upload tokens of its own.
//
// Usage:
//
//	relkit-console init -store URL [-agent URL] [-state FILE] [-out DIR] [-force]
//	relkit-console init -reset-admin [-state FILE] [-out DIR]
func runInit(out io.Writer, args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	storeURL := fs.String("store", "", "relkit-store base URL the panel reads (required)")
	target := fs.String("out", ".", "where to write the config")
	agentURL := fs.String("agent", "", "agent server base URL this panel manages")
	statePath := fs.String("state", "", "panel state file (default "+defaultPanelStatePath+")")
	force := fs.Bool("force", false, "overwrite an existing config")
	resetAdmin := fs.Bool("reset-admin", false, "regenerate the panel cookie key; every session is signed out")
	fs.Parse(args)

	outDir := *target
	configPath := filepath.Join(outDir, ConfigName)

	// The reset path runs before MkdirAll: bringing /etc/relkit-console into
	// existence is the panel's job on a fresh box, not the break-glass path on
	// a broken one.
	if *resetAdmin {
		if *force {
			return fmt.Errorf("-reset-admin cannot be combined with -force")
		}
		return runInitResetAdmin(out, configPath, *statePath)
	}

	if *storeURL == "" {
		return fmt.Errorf("-store is required: the panel reads relkit-store over HTTP")
	}
	if _, err := newStoreAdapter("init", *storeURL); err != nil {
		return err
	}
	panelPath, err := resolveAdminPath(*statePath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(configPath); err == nil && !*force {
		return fmt.Errorf("%s already exists; pass -force to overwrite "+
			"(this signs every operator out)", configPath)
	}

	// Config first, panel state second. A crash between the two leaves a config
	// with no cookie key: the panel mints one on first start.
	if err := os.WriteFile(configPath, skeletonBytes(*storeURL, *agentURL, panelPath), 0o644); err != nil {
		return err
	}
	doc, err := mintPanelDoc()
	if err != nil {
		return err
	}
	if err := writePanelDoc(panelPath, doc); err != nil {
		return err
	}

	fmt.Fprintf(out, "config %s\n", configPath)
	printPanelNote(out, panelPath)
	fmt.Fprintf(out, "\nPanel reads %s over HTTP; it never opens the release tree.\n", *storeURL)
	fmt.Fprintf(out, "Then: relkit-console -config %s\n", configPath)
	return nil
}

// runInitResetAdmin replaces the panel cookie key, signing every operator out.
// It does not touch accounts: those live on the agent now (requirement 13.6).
//
// This is the break-glass path the locked panel points at: it must work even
// when the config is gone, so a missing config falls back to the default state
// path rather than failing.
func runInitResetAdmin(out io.Writer, configPath, stateFlag string) error {
	configured := stateFlag
	if configured == "" {
		cfg, _, err := LoadFileConfig(configPath)
		if err != nil {
			return err
		}
		configured = adminStateFileFrom(cfg)
	}
	path, err := resolveAdminPath(configured)
	if err != nil {
		return err
	}
	doc, err := mintPanelDoc()
	if err != nil {
		return err
	}
	if err := writePanelDoc(path, doc); err != nil {
		return err
	}
	fmt.Fprintf(out, "config %s\n", configPath)
	printPanelNote(out, path)
	fmt.Fprintf(out, "\nEvery operator session is signed out. Restart to load it:\n")
	fmt.Fprintf(out, "  systemctl restart relkit-console\n")
	return nil
}

// hostOf extracts host:port from a URL for adapter naming; unparsable input
// falls back to the raw string so the name still says something.
func hostOf(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	return parsed.Host
}
