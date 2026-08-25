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
//	relkit-serve [flags]           run the server
//	relkit-serve init [dir]        write a config skeleton and a token
//	relkit-serve agent-guide       print the deployment guide
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
	uploadToken        []byte
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
		noCache       = flag.String("nocache", "index/,site/,latest/", "comma-separated prefixes served with no-cache")
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
	uploadToken, tokenWarnings, err := fileCfg.TokenFromFileConfig(usedPath)
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
		uploadToken = token
		warnings = append(warnings, checkPermissions(*tokenFile)...)
	} else if env := strings.TrimSpace(os.Getenv(tokenEnv)); env != "" {
		uploadToken = hashToken(env)
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
		uploadToken:   uploadToken,
		maxUpload:     maxUploadBytes,
		noCache:       splitPrefixes(*noCache),
		immutable:     splitPrefixes(*immutable),
		defaultMaxAge: *defaultMaxAge,
		logRequests:   !*quiet,
		gc:            newGCState(gcOn, *gcInterval, defaultGCDebounce),
		stats:         newDownloadStats(),
	}
	if fileCfg != nil {
		cfg.site = fileCfg.Site
		if fileCfg.Publish != nil {
			cfg.minPublishProtocol = fileCfg.Publish.MinProtocol
		}
	}

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
	if cfg.uploadToken != nil {
		log.Printf("PUT uploads enabled (max %s)", humanBytes(cfg.maxUpload))
	} else {
		log.Printf("read-only: no token supplied, PUT returns 405")
	}
	if gcOn {
		log.Printf("gc enabled (interval %s)", *gcInterval)
	} else {
		log.Printf("gc disabled")
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
	fs.Parse(args)

	outDir := *target
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	configPath := filepath.Join(outDir, ConfigName)
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
