// Command relkit-store is the storage plane split out of relkit-serve
// (ADR 0016): the relkit-compatible data plane and nothing else.
//
// It does three things: serve a release tree over HTTP with correct Range
// support so clients can download in parallel, accept authenticated PUT
// uploads so a publisher can push releases directly, and keep the tree tidy
// with CAS minting and orphan GC. The operator panel that used to live here
// is now relkit-console; this binary deliberately has no login, no accounts,
// and no human-facing pages beyond the raw directory listing.
//
// Usage:
//
//	relkit-store [flags]                  run the storage plane
//	relkit-store init [dir]               write config; called by deploy scripts
//	relkit-store -version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.firoyang.com/relkit/internal/publishproto"
)

// tokenEnv is the legacy name, kept so a systemd drop-in that rotated tokens
// for relkit-serve keeps working when the box switches binaries.
const tokenEnv = "RELKIT_SERVE_TOKEN"

// version is injected at build time with -ldflags "-X main.version=...".
var version = "dev"

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
	casSecret          []byte
	stats              *downloadStats
	minPublishProtocol int
	maxPublishProtocol int
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		if err := runInit(os.Stdout, os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
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
		gcEnabled     = flag.Bool("gc", true, "enable orphan manifest/artifact/cas cleanup")
		gcInterval    = flag.Duration("gc-interval", defaultGCInterval, "how often to sweep unreferenced objects; 0 disables GC")
		gcCASGrace    = flag.Duration("gc-cas-grace", defaultCASGrace, "keep unreferenced cas/ objects younger than this")
		showVersion   = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("relkit-store %s\n", version)
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
			if fileCfg.GC.CasGrace != "" && !explicit["gc-cas-grace"] {
				parsed, err := ParseDuration(fileCfg.GC.CasGrace)
				if err != nil {
					log.Fatalf("config: gc.casGrace: %v", err)
				}
				*gcCASGrace = parsed
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

	var casSecret []byte
	if len(credentials) > 0 {
		casSecret, err = loadOrCreateCASSecret(root.Name())
		if err != nil {
			log.Fatalf("cas secret: %v", err)
		}
	}

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
		gc:            newGCState(gcOn, *gcInterval, defaultGCDebounce, *gcCASGrace),
		casSecret:     casSecret,
		stats:         newDownloadStats(resolveStatsPath(root.Name(), statsFileFrom(fileCfg)), root.Name()),
	}
	if fileCfg != nil {
		if fileCfg.Publish != nil {
			cfg.minPublishProtocol = fileCfg.Publish.MinProtocol
			if fileCfg.Publish.MaxProtocol > 0 {
				cfg.maxPublishProtocol = fileCfg.Publish.MaxProtocol
			} else if fileCfg.Publish.MinProtocol > 0 {
				cfg.maxPublishProtocol = publishproto.Max
			}
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

	log.Printf("relkit-store %s", version)
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
