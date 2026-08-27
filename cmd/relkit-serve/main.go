// Command relkit-serve is a static file server for RUP release trees.
//
// It does two things: serve a directory over HTTP with correct Range support so
// that clients can download in parallel, and optionally accept authenticated
// PUT uploads so that a publisher can push a release to it directly.
//
// Range support is what "multi-threaded download" actually requires. A client
// opens several connections, each asking for a byte range, and the server
// answers each with 206 Partial Content. net/http's ServeContent implements
// that, including conditional and multi-range requests, so the work here is
// everything around it: staying inside the served directory, getting cache
// headers right for mutable versus immutable paths, and not breaking the
// zero-copy send path.
//
// Usage:
//
//	relkit-serve [flags]                  run the server
//	relkit-serve init [dir]               write a config skeleton and a token
//	relkit-serve init -product <id>       add or rotate a product upload token
//	relkit-serve init -product <id> -share-with <id>  attach to an existing token
//	relkit-serve init -list-products      list the product upload tokens
//	relkit-serve init -product <id> -remove   revoke one product
//	relkit-serve agent-guide              print the deployment guide
//	relkit-serve -version
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"cnb.cool/shichao402/relkit/internal/model"
)

const tokenEnv = "RELKIT_SERVE_TOKEN"

// version is injected at build time with -ldflags "-X main.version=...".
// Without it there is no way to tell which binary a box is actually running,
// which is the first thing anyone asks when behaviour differs between hosts.
var version = "dev"

// The guide travels inside the binary so that it can never drift from the
// build being run, and so that an agent on a fresh machine can read it without
// network access or a checked-out repository.
//
//go:embed AGENT-GUIDE.md
var agentGuide string

type config struct {
	root               *os.Root
	rootPath           string
	credentials        []credential
	maxUpload          int64
	noCache            []string
	immutable          []string
	defaultMaxAge      int
	logRequests        bool
	gc                 *gcState
	site               *SiteConfig
	stats              *downloadStats
	minPublishProtocol int
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
		case "agent-guide":
			fmt.Print(agentGuide)
			return
		}
	}
	runServer()
}

func runServer() {
	var (
		configPath    = flag.String("config", "", "path to "+ConfigName)
		addr          = flag.String("addr", ":8080", "address to listen on")
		dir           = flag.String("dir", ".", "directory to serve")
		tokenFile     = flag.String("token-file", "", "file holding the upload token; enables PUT")
		maxUpload     = flag.String("max-upload", "4GiB", "largest accepted upload")
		noCache       = flag.String("nocache", "index/,site/,latest/,browse/", "comma-separated prefixes served with no-cache")
		immutable     = flag.String("immutable", "manifest/,artifact/", "comma-separated prefixes served as immutable")
		defaultMaxAge = flag.Int("default-max-age", 60, "max-age for paths matching neither list")
		shutdownWait  = flag.Duration("shutdown-timeout", 30*time.Second, "how long to let in-flight downloads finish on shutdown")
		quiet         = flag.Bool("quiet", false, "do not log requests")
		gcEnabled     = flag.Bool("gc", true, "enable orphan manifest/artifact cleanup")
		gcInterval    = flag.Duration("gc-interval", defaultGCInterval, "how often to sweep unreferenced objects; 0 disables GC")
		showVersion   = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("relkit-serve %s\n", version)
		return
	}

	// Which flags the operator actually typed. Everything else is a default and
	// may be replaced by the config file.
	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	fileCfg, usedPath, err := LoadFileConfig(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	var warnings []string
	credentials, tokenWarnings, err := fileCfg.CredentialsFromFileConfig(usedPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	warnings = append(warnings, tokenWarnings...)

	if fileCfg != nil {
		if fileCfg.Addr != "" && !explicit["addr"] {
			*addr = fileCfg.Addr
		}
		if fileCfg.Dir != "" && !explicit["dir"] {
			*dir = fileCfg.Dir
		}
		if fileCfg.MaxUpload != "" && !explicit["max-upload"] {
			*maxUpload = fileCfg.MaxUpload
		}
		if fileCfg.ShutdownTimeout != "" && !explicit["shutdown-timeout"] {
			parsed, err := ParseDuration(fileCfg.ShutdownTimeout)
			if err != nil {
				log.Fatalf("config: shutdownTimeout: %v", err)
			}
			*shutdownWait = parsed
		}
		if fileCfg.LogRequests != nil && !explicit["quiet"] {
			*quiet = !*fileCfg.LogRequests
		}
		if fileCfg.Cache != nil {
			if len(fileCfg.Cache.NoCache) > 0 && !explicit["nocache"] {
				*noCache = strings.Join(fileCfg.Cache.NoCache, ",")
			}
			if len(fileCfg.Cache.Immutable) > 0 && !explicit["immutable"] {
				*immutable = strings.Join(fileCfg.Cache.Immutable, ",")
			}
			if fileCfg.Cache.DefaultMaxAge != nil && !explicit["default-max-age"] {
				*defaultMaxAge = *fileCfg.Cache.DefaultMaxAge
			}
		}
		if fileCfg.GC != nil {
			if fileCfg.GC.Enabled != nil && !explicit["gc"] {
				*gcEnabled = *fileCfg.GC.Enabled
			}
			if fileCfg.GC.Interval != "" && !explicit["gc-interval"] {
				parsed, err := ParseDuration(fileCfg.GC.Interval)
				if err != nil {
					log.Fatalf("config: gc.interval: %v", err)
				}
				*gcInterval = parsed
			}
		}
	}

	// Precedence for the token: command line, then environment, then config
	// file. The environment sits in the middle so that a container or a systemd
	// drop-in can rotate the token without rewriting a file that may be
	// managed by configuration management.
	if *tokenFile != "" {
		token, err := loadTokenFile(*tokenFile)
		if err != nil {
			log.Fatalf("upload token: %v", err)
		}
		credentials, err = withOperatorToken(credentials, token)
		if err != nil {
			log.Fatalf("upload token: %v", err)
		}
		warnings = append(warnings, checkPermissions(*tokenFile)...)
	} else if env := strings.TrimSpace(os.Getenv(tokenEnv)); env != "" {
		var err error
		credentials, err = withOperatorToken(credentials, hashToken(env))
		if err != nil {
			log.Fatalf("upload token: %v", err)
		}
	}

	maxUploadBytes, err := ParseSize(*maxUpload)
	if err != nil {
		log.Fatalf("max-upload: %v", err)
	}

	root, err := os.OpenRoot(*dir)
	if err != nil {
		log.Fatalf("cannot serve %s: %v", *dir, err)
	}
	defer root.Close()

	gcOn := *gcEnabled && *gcInterval > 0
	cfg := &config{
		root:          root,
		rootPath:      root.Name(),
		credentials:   credentials,
		maxUpload:     maxUploadBytes,
		noCache:       splitPrefixes(*noCache),
		immutable:     splitPrefixes(*immutable),
		defaultMaxAge: *defaultMaxAge,
		logRequests:   !*quiet,
		gc:            newGCState(gcOn, *gcInterval, defaultGCDebounce),
		stats:         newDownloadStats(resolveStatsPath(root.Name(), statsFileFrom(fileCfg)), root.Name()),
	}
	if fileCfg != nil {
		cfg.site = fileCfg.Site
		if fileCfg.Publish != nil {
			cfg.minPublishProtocol = fileCfg.Publish.MinProtocol
		}
	}
	defer cfg.stats.stop()

	srv := &http.Server{
		Addr:    *addr,
		Handler: cfg.handler(),

		// ReadHeaderTimeout alone bounds slow-header attacks. WriteTimeout is
		// deliberately left unset: it applies to the whole response, so any
		// value large enough for a 200 MB download over a slow link is too
		// large to protect anything, and any value small enough to protect
		// something would kill legitimate downloads partway through. Idle
		// connections are bounded by IdleTimeout instead.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("cannot listen on %s: %v", *addr, err)
	}

	log.Printf("relkit-serve %s", version)
	if usedPath != "" {
		log.Printf("config: %s", usedPath)
	} else {
		log.Printf("config: none (flags and defaults only)")
	}
	log.Printf("serving %s on http://%s", cfg.rootPath, listener.Addr())
	if cfg.uploadsEnabled() {
		log.Printf("PUT uploads enabled (max %s)", humanBytes(cfg.maxUpload))
	} else {
		log.Printf("read-only: no token supplied, PUT returns 405")
	}
	if gcOn {
		log.Printf("gc enabled (interval %s)", *gcInterval)
	} else {
		log.Printf("gc disabled")
	}
	if cfg.stats != nil && cfg.stats.path != "" {
		log.Printf("download stats: %s (since %s)", cfg.stats.path, cfg.stats.startedAt())
	}
	for _, warning := range warnings {
		log.Printf("WARNING: %s", warning)
	}

	gcStop := make(chan struct{})
	cfg.startGCLoop(gcStop)

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(listener) }()

	select {
	case err := <-errc:
		close(gcStop)
		cfg.stopGC()
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server stopped: %v", err)
		}
	case <-ctx.Done():
		close(gcStop)
		cfg.stopGC()
		log.Printf("shutting down, waiting up to %s for in-flight downloads", *shutdownWait)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), *shutdownWait)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			// Clients cut off here retry with a Range request and resume, which
			// is the same path they take through any network interruption.
			log.Printf("forced shutdown with downloads still running: %v", err)
		}
	}
}

// runInit writes a config skeleton plus a freshly generated token, so that a
// working deployment needs no hand-written secrets and no guessing about which
// cache prefixes matter.
func runInit(out io.Writer, args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	dir := fs.String("dir", "/srv/releases", "directory the server will serve")
	target := fs.String("out", ".", "where to write the config and token")
	force := fs.Bool("force", false, "overwrite existing files")
	tokenOnly := fs.Bool("token-only", false, "generate a new token and leave the config alone")
	product := fs.String("product", "", "add or rotate a product-scoped upload token")
	shareWith := fs.String("share-with", "", "with -product: grant the same token as this existing product")
	list := fs.Bool("list-products", false, "list the product-scoped upload tokens and exit")
	remove := fs.Bool("remove", false, "with -product: revoke that product's upload token")
	fs.Parse(args)

	outDir := *target
	configPath := filepath.Join(outDir, ConfigName)

	// Listing, revoking, and sharing run before MkdirAll: none of them should
	// bring into existence the directory it was pointed at, which would turn a
	// typo in -out into a silent "no products configured".
	switch {
	case *list:
		if *product != "" || *remove || *shareWith != "" {
			return fmt.Errorf("-list-products takes no other arguments")
		}
		return runInitListProducts(out, configPath)
	case *remove:
		if *product == "" {
			return fmt.Errorf("-remove needs -product <id>")
		}
		if *tokenOnly || *force || *shareWith != "" {
			return fmt.Errorf("-remove cannot be combined with -token-only, -force, or -share-with")
		}
		return runInitRemoveProduct(out, configPath, *product)
	}
	if *shareWith != "" {
		if *product == "" {
			return fmt.Errorf("-share-with needs -product <id>")
		}
		if *tokenOnly || *force {
			return fmt.Errorf("-share-with cannot be combined with -token-only or -force")
		}
		return runInitShareProduct(out, configPath, *product, *shareWith)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	if *product != "" {
		return runInitProduct(out, outDir, *dir, *product, *force, *tokenOnly)
	}

	tokenPath := filepath.Join(outDir, "relkit-serve.token")

	// Rotating a token must not touch the config. Anything hand-edited there
	// (listen address, cache prefixes) would be silently reverted to defaults,
	// and a reverted cache prefix shows up only as releases arriving late.
	written := []string{tokenPath}
	if !*tokenOnly {
		written = append(written, configPath)
	}
	for _, path := range written {
		if _, err := os.Stat(path); err == nil && !*force && !*tokenOnly {
			return fmt.Errorf("%s already exists; pass -force to overwrite "+
				"(this invalidates the current token)", path)
		}
	}

	token, err := generateToken()
	if err != nil {
		return err
	}

	// 0600 from the start rather than written then tightened: between those two
	// steps the token would be world-readable, and on a shared box that is long
	// enough.
	if err := os.WriteFile(tokenPath, []byte(token+"\n"), 0o600); err != nil {
		return err
	}
	if !*tokenOnly {
		if err := os.WriteFile(configPath, Skeleton(*dir), 0o644); err != nil {
			return err
		}
	}

	if *tokenOnly {
		fmt.Fprintf(out, "token  %s (mode 0600, replaced)\n", tokenPath)
		fmt.Fprintf(out, "\nRestart the service to load it. Every publisher needs "+
			"the new value first:\n")
		fmt.Fprintf(out, "  export RELKIT_UPLOAD_TOKEN='%s'\n", token)
		return nil
	}

	fmt.Fprintf(out, "config %s\n", configPath)
	fmt.Fprintf(out, "token  %s (mode 0600)\n", tokenPath)
	fmt.Fprintf(out, "\nThe publisher needs this token in its environment:\n")
	fmt.Fprintf(out, "  export RELKIT_UPLOAD_TOKEN='%s'\n\n", token)
	fmt.Fprintf(out, "Serving directory is %s; create it before starting.\n", *dir)
	fmt.Fprintf(out, "Then: relkit-serve -config %s\n", configPath)
	fmt.Fprintf(out, "Deployment steps and troubleshooting: relkit-serve agent-guide\n")
	return nil
}

// runInitShareProduct grants product the same upload secret as with, without
// minting a new token. Family products that already share a publisher can
// write their own trees with one CI secret; the server still isolates paths
// by product id. There is no plaintext to print: the existing hash cannot be
// reversed.
func runInitShareProduct(out io.Writer, configPath, product, with string) error {
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return err
	}
	if err := model.CheckIdentifier(with, "product"); err != nil {
		return err
	}
	if product == with {
		return fmt.Errorf("-product and -share-with must be different ids")
	}

	cfg, err := loadInitConfig(configPath)
	if err != nil {
		return err
	}
	relFile, err := cfg.shareProductToken(product, with)
	if err != nil {
		return err
	}
	if err := writeFileConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "config %s\n", configPath)
	fmt.Fprintf(out, "token  %s (shared; products now include %s)\n", relFile, product)
	fmt.Fprintf(out, "\nNo new secret. Reuse the RELKIT_UPLOAD_TOKEN already held for %s.\n", with)
	fmt.Fprintf(out, "Restart the service to load it:\n")
	fmt.Fprintf(out, "  systemctl restart relkit-serve\n")
	return nil
}

func runInitProduct(out io.Writer, outDir, serveDir, product string, force, tokenOnly bool) error {
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return err
	}

	configPath := filepath.Join(outDir, ConfigName)
	relFile := productTokenRelPath(product)
	tokenPath := filepath.Join(outDir, filepath.FromSlash(relFile))

	if tokenOnly {
		cfg, _, err := LoadFileConfig(configPath)
		if err != nil {
			return err
		}
		rel, ok := cfg.productTokenFile(product)
		if !ok {
			return fmt.Errorf("%s is not listed in uploadTokens; omit -token-only to add it", product)
		}
		path := resolveRelative(rel, configPath)
		token, err := writeTokenFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "token  %s (mode 0600, replaced)\n", path)
		fmt.Fprintf(out, "\nRestart the service to load it. This product's publisher needs "+
			"the new value first:\n")
		fmt.Fprintf(out, "  export RELKIT_UPLOAD_TOKEN='%s'\n", token)
		return nil
	}

	if _, err := os.Stat(tokenPath); err == nil && !force {
		return fmt.Errorf("%s already exists; pass -force to overwrite "+
			"(this invalidates the current token) or -token-only to rotate", tokenPath)
	}

	token, err := writeTokenFile(tokenPath)
	if err != nil {
		return err
	}

	createdOperator := ""
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, Skeleton(serveDir), 0o644); err != nil {
			return err
		}
		operatorPath := filepath.Join(outDir, "relkit-serve.token")
		if _, err := os.Stat(operatorPath); os.IsNotExist(err) {
			op, err := writeTokenFile(operatorPath)
			if err != nil {
				return err
			}
			createdOperator = op
		}
	} else if err != nil {
		return err
	}

	cfg, _, err := LoadFileConfig(configPath)
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("config %s is empty", configPath)
	}
	cfg.upsertProductToken(product, relFile)
	if err := writeFileConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "config %s\n", configPath)
	fmt.Fprintf(out, "token  %s (mode 0600, product %s)\n", tokenPath, product)
	fmt.Fprintf(out, "\nThis product's publisher needs this token in its environment:\n")
	fmt.Fprintf(out, "  export RELKIT_UPLOAD_TOKEN='%s'\n", token)
	if createdOperator != "" {
		fmt.Fprintf(out, "\nAn operator token was also written (full-tree PUT):\n")
		fmt.Fprintf(out, "  export RELKIT_SERVE_TOKEN='%s'\n", createdOperator)
	}
	return nil
}

// runInitListProducts reports which products this box will accept uploads for.
//
// Ids and file paths only. An inventory that printed the tokens it inventories
// could not be run in front of anyone, and the point of listing is to answer
// "is this product still allowed", not "what is its secret".
func runInitListProducts(out io.Writer, configPath string) error {
	cfg, err := loadInitConfig(configPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "config %s\n", configPath)
	switch {
	case cfg.UploadToken != "":
		fmt.Fprintf(out, "operator  inline in config (full tree)\n")
	case cfg.UploadTokenFile != "":
		fmt.Fprintf(out, "operator  %s (full tree)\n", cfg.UploadTokenFile)
	default:
		fmt.Fprintf(out, "operator  none\n")
	}
	if len(cfg.UploadTokens) == 0 {
		fmt.Fprintf(out, "products  none\n")
		return nil
	}
	fmt.Fprintf(out, "products\n")
	for _, entry := range cfg.UploadTokens {
		fmt.Fprintf(out, "  %-24s %s\n", strings.Join(entry.Products, ","), entry.File)
	}
	return nil
}

// runInitRemoveProduct revokes a product-scoped token: the entry leaves
// uploadTokens and its token file is deleted. Everything else in the config,
// including the operator token and the cache prefixes, is left as written.
func runInitRemoveProduct(out io.Writer, configPath, product string) error {
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return err
	}
	cfg, err := loadInitConfig(configPath)
	if err != nil {
		return err
	}
	relFile, ok := cfg.removeProductToken(product)
	if !ok {
		return fmt.Errorf("%s is not listed in uploadTokens", product)
	}

	// Config first, file second. In the other order a crash in between leaves
	// the config pointing at a token file that no longer exists, and the
	// server refuses to start at all rather than starting without one product.
	if err := writeFileConfig(configPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(out, "config %s (product %s removed)\n", configPath, product)

	if relFile == "" {
		fmt.Fprintf(out, "token  kept: that file is shared with another product\n")
	} else {
		path := resolveRelative(relFile, configPath)
		switch err := os.Remove(path); {
		case err == nil:
			fmt.Fprintf(out, "token  %s deleted\n", path)
		case os.IsNotExist(err):
			fmt.Fprintf(out, "token  %s was already gone\n", path)
		default:
			return err
		}
	}

	fmt.Fprintf(out, "\nThe running process keeps accepting the old token until it reloads:\n")
	fmt.Fprintf(out, "  systemctl restart relkit-serve\n")
	return nil
}

func loadInitConfig(configPath string) (*FileConfig, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%s does not exist; run relkit-serve init first", configPath)
	}
	cfg, _, err := LoadFileConfig(configPath)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("config %s is empty", configPath)
	}
	return cfg, nil
}

func writeTokenFile(path string) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	return token, nil
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// loadTokenFile reads a token from a file.
//
// There is deliberately no flag that takes the token itself: it would be
// visible in `ps` to every user on the box and would land in shell history.
func loadTokenFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return nil, fmt.Errorf("%s is empty", path)
	}
	return hashToken(token), nil
}

// hashToken reduces the token to a fixed-size digest so that comparison is
// constant-time in a way that cannot leak the length either.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func splitPrefixes(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value)
}
