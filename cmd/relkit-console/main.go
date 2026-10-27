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
	"strings"
	"time"
)

// Command relkit-console is the operator panel binary split out of
// relkit-serve (ADR 0016). It serves the management plane: the JSON API behind
// the single-page front end, the app shell, the fixed download URLs and the
// health endpoint.
//
// It serves no uploads and publishes nothing. Every storage fact arrives
// through the tree adapter (adapters.go), chosen by the "tree" entry of the
// config; writes are forwarded to relkit-agent under the operator's own
// bearer. Pointing a browser at the console cannot publish anything.
//
// Usage:
//
//	relkit-console [-config PATH]                     run the panel
//	relkit-console init -tree SPEC [-out DIR]         write a panel config and
//	                                                  the panel's cookie key
//	relkit-console init -reset-admin                  issue a new cookie key;
//	                                                  every session signs out
//	relkit-console -version
//
// Config (relkit-console.json, next to the binary or /etc):
//
//	{
//	  "addr": "127.0.0.1:8081",
//	  "tree": {"type": "...", ...},              // the tree adapter, see adapters.go
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
	Tree           AdapterSpec `json:"tree"`
	Agent          string      `json:"agent,omitempty"`
	AdminStateFile string      `json:"adminStateFile,omitempty"`
	Site           *SiteConfig `json:"site,omitempty"`
}

// retiredConfigKeys are top-level keys older consoles wrote. They named one
// adapter implementation's parameters at the top of the config; the deploy
// script's upgrade rewrites them into a "tree" entry.
var retiredConfigKeys = []string{"store", "dir", "stateDir"}

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
	if err := rejectRetiredKeys(raw); err != nil {
		return nil, path, fmt.Errorf("%s: %w", path, err)
	}
	var cfg FileConfig
	if err := jsonDecodeStrict(raw, &cfg); err != nil {
		return nil, path, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, path, nil
}

// rejectRetiredKeys turns strict decode's generic "unknown field" into an
// error that says how to fix the file.
func rejectRetiredKeys(raw []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil // strict decode reports the syntax error
	}
	var found []string
	for _, key := range retiredConfigKeys {
		if _, ok := top[key]; ok {
			found = append(found, key)
		}
	}
	if len(found) == 0 {
		return nil
	}
	return fmt.Errorf("retired config keys %s: the tree is configured by a \"tree\" entry now; "+
		"run scripts/deploy/relkit.py upgrade to migrate this file", strings.Join(found, ", "))
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
		treeFlag    = flag.String("tree", "", "tree adapter spec as JSON, or @FILE (overrides the config's \"tree\")")
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
	var tree AdapterSpec
	if fileCfg != nil {
		if fileCfg.Addr != "" && !explicit["addr"] {
			*addr = fileCfg.Addr
		}
		tree = fileCfg.Tree
		if fileCfg.Agent != "" && !explicit["agent"] {
			*agentAddr = fileCfg.Agent
		}
	}
	if explicit["tree"] {
		tree, err = readAdapterSpec(*treeFlag)
		if err != nil {
			log.Fatalf("-tree: %v", err)
		}
	}

	adapter, err := newAdapter(tree)
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
	log.Printf("panel on http://%s reading the tree via %s", *addr, adapter.Name())
	log.Printf("panel state: %s", adminPath)
	log.Printf("panel: %s", admin.statusLog())

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}

// readAdapterSpec accepts a spec written inline or as @FILE. The spec is kept
// as written; only newAdapter interprets it.
func readAdapterSpec(arg string) (AdapterSpec, error) {
	raw := []byte(strings.TrimSpace(arg))
	if strings.HasPrefix(arg, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(arg, "@"))
		if err != nil {
			return nil, err
		}
		raw = []byte(strings.TrimSpace(string(data)))
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty spec")
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("spec is not valid JSON")
	}
	return AdapterSpec(raw), nil
}

// skeletonBytes renders the panel config a fresh box starts from. Only the
// endpoints matter; the site section is left for the operator to shape.
func skeletonBytes(tree AdapterSpec, agent, statePath string) []byte {
	cfg := &FileConfig{
		Addr:           "127.0.0.1:8081",
		Tree:           tree,
		Agent:          agent,
		AdminStateFile: statePath,
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		panic(err) // the spec was validated as JSON before reaching here
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
//	relkit-console init -tree SPEC [-agent URL] [-state FILE] [-out DIR] [-force]
//	relkit-console init -reset-admin [-state FILE] [-out DIR]
func runInit(out io.Writer, args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	treeArg := fs.String("tree", "", "tree adapter spec as JSON, or @FILE (required)")
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

	if strings.TrimSpace(*treeArg) == "" {
		return fmt.Errorf("-tree is required: the panel reads the tree through a tree adapter "+
			"(available types: %s)", strings.Join(AdapterTypes(), ", "))
	}
	tree, err := readAdapterSpec(*treeArg)
	if err != nil {
		return fmt.Errorf("-tree: %w", err)
	}
	adapter, err := newAdapter(tree)
	if err != nil {
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
	if err := os.WriteFile(configPath, skeletonBytes(tree, *agentURL, panelPath), 0o644); err != nil {
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
	fmt.Fprintf(out, "\nPanel reads the tree via %s; it never opens the release tree directly.\n", adapter.Name())
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
