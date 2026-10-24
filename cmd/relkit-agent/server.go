package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/shichao402/relkit/internal/agentadmin"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/custody"
	"github.com/shichao402/relkit/internal/directory"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/humansize"
	"github.com/shichao402/relkit/internal/makers"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/publish"
	"github.com/shichao402/relkit/internal/publishproto"
	siterebuild "github.com/shichao402/relkit/internal/site"
	"github.com/shichao402/relkit/internal/stage"
	"github.com/shichao402/relkit/internal/verify"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	vnextproject "github.com/shichao402/relkit/internal/vnext/project"
	vnextrelease "github.com/shichao402/relkit/internal/vnext/release"
	vnextruntimeprofile "github.com/shichao402/relkit/internal/vnext/runtimeprofile"
)

const errLegacyProductConfig = "products and upload tokens are no longer config-file fields: register the product and issue a token through the admin surface (requirements 11.3, 13.6)"

// ProductConfig is one registered product's on-host layout. Registration itself
// lives in the admin store; this type is only how the server hands a product to
// the code paths that need its tree.
type ProductConfig struct {
	Root    string `json:"root"`
	Profile string `json:"profile,omitempty"`
}

// FileConfig is the deployment-level configuration of one agent server. It
// deliberately holds no product registration and no credential: products and
// publisher tokens are agent state, managed through the admin API and stored
// under stateDir (requirements 11.3, 13.6). A config file that still names
// products is rejected at load time rather than silently ignored.
type FileConfig struct {
	Addr string `json:"addr"`
	// RootKeyID names this server's one root in the custody manifest. The
	// manifest resolves it to a carrier, so the config never learns a key path
	// (requirements 10.1, 15.3).
	RootKeyID string `json:"rootKeyId,omitempty"`
	// CustodyManifest is the custody manifest this server reads its root from.
	CustodyManifest string `json:"custodyManifest,omitempty"`
	// AdminStateFile is where the management document lives (users, roles,
	// products, token hashes). Unset means <stateDir>/admin.json.
	AdminStateFile     string `json:"adminStateFile,omitempty"`
	MaxUpload          string `json:"maxUpload,omitempty"`
	MaxFiles           int    `json:"maxFiles,omitempty"`
	PartSize           string `json:"partSize,omitempty"`
	MinPartSize        string `json:"minPartSize,omitempty"`
	MaxPartSize        string `json:"maxPartSize,omitempty"`
	MaxParts           int    `json:"maxParts,omitempty"`
	MaxPartConcurrency int    `json:"maxPartConcurrency,omitempty"`
	UploadTTL          string `json:"uploadTTL,omitempty"`
	StateDir           string `json:"stateDir,omitempty"`
	// ToolsDir holds the released relkit tool files this server distributes.
	// Unset means <stateDir>/tools.
	ToolsDir string `json:"toolsDir,omitempty"`
	// ToolsTokenFile is the deploy pipeline's credential for publishing tools.
	// Tool publishing is a deployment action, not a product action: one server
	// has one toolchain and one deploy pipeline (requirement 12.6), so it must
	// not ride on a product upload token. Empty disables tool publishing.
	ToolsTokenFile     string `json:"toolsTokenFile,omitempty"`
	// PublicURL is the address hosts should put in their project config. Unset
	// means the management API derives it from the request that asked.
	PublicURL          string `json:"publicURL,omitempty"`
	// ProductRootParent is where a product registered without an explicit root
	// gets its tree: <productRootParent>/<id>. Unset means the agent infers it
	// from the products already registered; there is no hardcoded fallback,
	// because a guessed path the service user cannot create only fails later.
	ProductRootParent  string `json:"productRootParent,omitempty"`
	MinPublishProtocol *int   `json:"minPublishProtocol,omitempty"`
	MaxPublishProtocol *int   `json:"maxPublishProtocol,omitempty"`
	Site               SiteConfig `json:"site,omitempty"`
}


type SiteConfig struct {
	// Makers is the pre-sinks spelling (ADR 0015 deprecated). It expands to a
	// single {"type":"makers"} sink at load time and must not be combined
	// with sinks.
	Makers *makers.Config `json:"makers,omitempty"`
	// Sinks is the declarative browse-dump destination list (ADR 0015).
	Sinks []siterebuild.SinkSpec `json:"sinks,omitempty"`
}

type Config struct {
	Addr string
	// RootKeyID is this server's one signing identity, named in the custody
	// manifest. Root is the resolved material and is never serialized.
	RootKeyID          string
	CustodyManifest    string
	Root               *config.RootKey
	admin              *agentadmin.Store
	credentials        []agentadmin.Credential
	MaxUpload          int64
	MaxFiles           int

	PartSize           int64
	MinPartSize        int64
	MaxPartSize        int64
	MaxParts           int
	MaxPartConcurrency int
	UploadTTL          time.Duration
	StateDir           string
	// ToolsDir is where distributable relkit tool files live; empty means
	// <StateDir>/tools.
	ToolsDir string
	// ToolsTokenHash gates POST /v1/tools. Nil means tool publishing is off,
	// which is the safe default for a read-only mirror.
	ToolsTokenHash     []byte
	MinPublishProtocol int
	MaxPublishProtocol int
	// PublicURL is what the panel prints in a connect config. Empty means the
	// management API derives it from the request.
	PublicURL  string
	// ProductRootParent is the configured parent for default product roots.
	// Empty means infer from existing registrations (see productRootParent).
	ProductRootParent string
	Site       SiteConfig
	Products   map[string]ProductConfig
	ConfigPath string
}

func LoadConfig(path string) (*Config, error) {
	raw, err := loadFileConfig(path)
	if err != nil {
		return nil, err
	}
	if raw.Site.Makers != nil && len(raw.Site.Sinks) > 0 {
		return nil, fmt.Errorf("site.makers and site.sinks are mutually exclusive; migrate site.makers to site.sinks [{\"type\":\"makers\",...}]")
	}
	if raw.Site.Makers != nil {
		if strings.TrimSpace(raw.Site.Makers.ProjectID) == "" {
			return nil, fmt.Errorf("site.makers.projectId is required")
		}
		switch raw.Site.Makers.Region {
		case "", "china", "global":
		default:
			return nil, fmt.Errorf(`site.makers.region must be "china" or "global"`)
		}
		if raw.Site.Makers.TokenEnv == "" {
			raw.Site.Makers.TokenEnv = makers.DefaultTokenEnv
		}
		if raw.Site.Makers.Region == "" {
			raw.Site.Makers.Region = "china"
		}
		raw.Site.Sinks = []siterebuild.SinkSpec{{
			Type: siterebuild.SinkMakers, ProjectID: raw.Site.Makers.ProjectID,
			Region: raw.Site.Makers.Region, TokenEnv: raw.Site.Makers.TokenEnv,
		}}
	}
	sinks, err := siterebuild.NormalizeSinks(raw.Site.Sinks)
	if err != nil {
		return nil, err
	}
	raw.Site.Sinks = sinks
	cfg := &Config{
		Addr:               raw.Addr,
		MaxUpload:          4 << 30,
		MaxFiles:           10_000,
		PartSize:           8 << 20,
		MinPartSize:        1 << 20,
		MaxPartSize:        64 << 20,
		MaxParts:           10_000,
		MaxPartConcurrency: 16,
		UploadTTL:          24 * time.Hour,
		StateDir:           raw.StateDir,
		ToolsDir:           raw.ToolsDir,
		MinPublishProtocol: publishproto.Min,
		MaxPublishProtocol: publishproto.Max,
		Site:               raw.Site,
		PublicURL:          strings.TrimSpace(raw.PublicURL),
		ConfigPath:         path,
	}
	if parent := strings.TrimSpace(raw.ProductRootParent); parent != "" {
		if !filepath.IsAbs(parent) {
			parent = filepath.Join(filepath.Dir(path), parent)
		}
		cfg.ProductRootParent = filepath.Clean(parent)
	}
	// Default to the contract this build speaks. An operator can lower Min to
	// ride out a publisher rollout, or set 0 to disable the handshake. Max
	// rejects publishers that speak a future contract this build does not.
	if raw.MinPublishProtocol != nil {
		cfg.MinPublishProtocol = *raw.MinPublishProtocol
	}
	if raw.MaxPublishProtocol != nil {
		cfg.MaxPublishProtocol = *raw.MaxPublishProtocol
	}
	if raw.MaxFiles > 0 {
		cfg.MaxFiles = raw.MaxFiles
	}
	if raw.MaxParts > 0 {
		cfg.MaxParts = raw.MaxParts
	}
	if raw.MaxPartConcurrency > 0 {
		cfg.MaxPartConcurrency = raw.MaxPartConcurrency
	}
	if raw.MaxUpload != "" {
		n, err := parseSize(raw.MaxUpload)
		if err != nil {
			return nil, err
		}
		cfg.MaxUpload = n
	}
	if raw.PartSize != "" {
		n, err := parseSize(raw.PartSize)
		if err != nil {
			return nil, fmt.Errorf("partSize: %w", err)
		}
		cfg.PartSize = n
	}
	if raw.MinPartSize != "" {
		n, err := parseSize(raw.MinPartSize)
		if err != nil {
			return nil, fmt.Errorf("minPartSize: %w", err)
		}
		cfg.MinPartSize = n
	}
	if raw.MaxPartSize != "" {
		n, err := parseSize(raw.MaxPartSize)
		if err != nil {
			return nil, fmt.Errorf("maxPartSize: %w", err)
		}
		cfg.MaxPartSize = n
	}
	if raw.UploadTTL != "" {
		d, err := time.ParseDuration(raw.UploadTTL)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("uploadTTL: invalid duration %q", raw.UploadTTL)
		}
		cfg.UploadTTL = d
	}
	if cfg.MinPartSize < 1 || cfg.MaxPartSize < cfg.MinPartSize || cfg.PartSize < cfg.MinPartSize || cfg.PartSize > cfg.MaxPartSize {
		return nil, fmt.Errorf("part sizes must satisfy 1 <= minPartSize <= partSize <= maxPartSize")
	}
	if cfg.StateDir == "" {
		cfg.StateDir = filepath.Join(filepath.Dir(path), "relkit-agent-state")
	}
	cfg.RootKeyID = strings.TrimSpace(raw.RootKeyID)
	if strings.TrimSpace(raw.CustodyManifest) != "" {
		manifestPath := raw.CustodyManifest
		if !filepath.IsAbs(manifestPath) {
			manifestPath = filepath.Join(filepath.Dir(path), manifestPath)
		}
		cfg.CustodyManifest = mustAbs(manifestPath)
	}
	// A server cannot sign anything without its root, and a root that does not
	// match its registered public key would emit documents nobody can verify.
	// Resolve it here so a bad manifest stops the process at startup instead of
	// failing on the first publish (requirements 10.1, 15.2).
	if cfg.RootKeyID != "" || cfg.CustodyManifest != "" {
		if cfg.RootKeyID == "" {
			return nil, fmt.Errorf("custodyManifest is set but rootKeyId is missing; name the keyId to read from the manifest")
		}
		if cfg.CustodyManifest == "" {
			return nil, fmt.Errorf("rootKeyId %q is set but custodyManifest is missing; a server reads its root only through the custody manifest", cfg.RootKeyID)
		}
		manifest, err := custody.Load(cfg.CustodyManifest)
		if err != nil {
			return nil, fmt.Errorf("custody manifest: %w", err)
		}
		root, err := manifest.Root(cfg.RootKeyID)
		if err != nil {
			return nil, fmt.Errorf("custody manifest: %w", err)
		}
		cfg.Root = &config.RootKey{
			KeyID:       root.KeyID,
			Seed:        root.Seed,
			PublicKey:   root.PublicKey,
			Fingerprint: root.Fingerprint,
		}
	}
	admin, err := openAdminStore(path, raw)
	if err != nil {
		return nil, err
	}
	cfg.admin = admin
	if err := refreshAdminState(cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw.ToolsTokenFile) != "" {
		toolTokenPath := raw.ToolsTokenFile
		if !filepath.IsAbs(toolTokenPath) {
			toolTokenPath = filepath.Join(filepath.Dir(path), toolTokenPath)
		}
		data, err := os.ReadFile(toolTokenPath)
		if err != nil {
			return nil, fmt.Errorf("toolsTokenFile: %w", err)
		}
		token := strings.TrimSpace(string(data))
		if token == "" {
			return nil, fmt.Errorf("toolsTokenFile: %s is empty", toolTokenPath)
		}
		cfg.ToolsTokenHash = hashToken(token)
	}
	return cfg, nil
}

func loadFileConfig(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// The epoch upgrade keeps no reader for the old spelling. Silently ignoring
	// `products` or `uploadTokens` would leave a server that looks configured and
	// authorizes nobody, so a config file that still carries them is an error
	// (requirements 11.3, 13.6).
	var legacy map[string]json.RawMessage
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, err
	}
	for _, field := range []string{"products", "uploadTokens", "uploadToken", "uploadTokenFile"} {
		if _, found := legacy[field]; found {
			return nil, fmt.Errorf("%s: %q is gone: %s", path, field, errLegacyProductConfig)
		}
	}
	var raw FileConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &raw, nil
}

// openAdminStore opens the management document. Its location defaults to
// <stateDir>/admin.json so a deployment that never sets adminStateFile still has
// one place for users, products and token hashes.
func openAdminStore(path string, raw *FileConfig) (*agentadmin.Store, error) {
	adminPath := strings.TrimSpace(raw.AdminStateFile)
	if adminPath == "" {
		stateDir := raw.StateDir
		if stateDir == "" {
			stateDir = filepath.Join(filepath.Dir(path), "relkit-agent-state")
		}
		adminPath = filepath.Join(stateDir, "admin.json")
	} else if !filepath.IsAbs(adminPath) {
		adminPath = filepath.Join(filepath.Dir(path), adminPath)
	}
	store, err := agentadmin.Open(mustAbs(adminPath))
	if err != nil {
		return nil, fmt.Errorf("admin state: %w", err)
	}
	return store, nil
}

// loadAdminProducts projects the admin store's registrations into the on-host
// layout the request path needs. Relative roots and profiles are resolved
// against the config file, because registration records what the operator typed
// and the agent runs from wherever the service was started.
func loadAdminProducts(cfg *Config, admin *agentadmin.Store, configPath string) error {
	products := map[string]ProductConfig{}
	base := filepath.Dir(configPath)
	for _, registered := range admin.Products() {
		product := ProductConfig{Root: registered.Root, Profile: registered.Profile}
		if strings.TrimSpace(product.Root) == "" {
			return fmt.Errorf("product %q: root is required", registered.ID)
		}
		if !filepath.IsAbs(product.Root) {
			product.Root = filepath.Join(base, product.Root)
		}
		product.Root = mustAbs(product.Root)
		if strings.TrimSpace(product.Profile) == "" {
			product.Profile = filepath.Join(base, "products", registered.ID+".json")
		} else if !filepath.IsAbs(product.Profile) {
			product.Profile = filepath.Join(base, product.Profile)
		}
		product.Profile = mustAbs(product.Profile)
		products[registered.ID] = product
	}
	cfg.Products = products
	return nil
}

// refreshAdminState projects the admin document into the runtime view: the
// product map the request path reads and the live credential list. Both change
// at runtime, so this is the one function that keeps them in step.
func refreshAdminState(cfg *Config) error {
	if cfg.admin == nil {
		return nil
	}
	if err := loadAdminProducts(cfg, cfg.admin, cfg.ConfigPath); err != nil {
		return err
	}
	cfg.credentials = cfg.admin.Credentials()
	return nil
}

// reloadAdmin re-reads registrations and live credentials after a management
// write. Product and token changes take effect without a restart, which is what
// "issue a token in the panel" has to mean.
func (s *Server) reloadAdmin() error {
	if s.cfg.admin == nil {
		return nil
	}
	s.productMu.Lock()
	defer s.productMu.Unlock()
	return refreshAdminState(s.cfg)
}

func parseSize(text string) (int64, error) {
	return humansize.Parse(text)
}

type Server struct {
	cfg *Config
	// productMu guards cfg.Products and cfg.credentials. The management surface
	// changes both at runtime (register a product, issue or revoke a token), so
	// the request path cannot read them through an unsynchronized map.
	productMu     sync.RWMutex
	locks         sync.Map // product -> *sync.Mutex
	siteMu        sync.Mutex
	// allowEmptySite is set only by `site-rebuild -allow-empty`: it is the
	// operator's explicit permission to write a snapshot with no products, used
	// after retiring every product. Publish-triggered rebuilds never set it.
	allowEmptySite bool
	siteTriggerMu  sync.Mutex
	siteRunning    bool
	sitePending    bool
	idemMu         sync.Mutex
	uploads        sync.Map // upload id -> *liveUpload
}

// product returns one registration under the read lock.
func (s *Server) product(id string) (ProductConfig, bool) {
	s.productMu.RLock()
	defer s.productMu.RUnlock()
	product, ok := s.cfg.Products[id]
	return product, ok
}

// productIDs lists registered products under the read lock.
func (s *Server) productIDs() map[string]ProductConfig {
	s.productMu.RLock()
	defer s.productMu.RUnlock()
	out := make(map[string]ProductConfig, len(s.cfg.Products))
	for id, product := range s.cfg.Products {
		out[id] = product
	}
	return out
}

func NewServer(cfg *Config) *Server {
	// Direct construction (tests, embedders) bypasses LoadConfig; repeat the
	// site.makers → sinks expansion so the sink plane has one owner.
	if len(cfg.Site.Sinks) == 0 && cfg.Site.Makers != nil {
		cfg.Site.Sinks = []siterebuild.SinkSpec{{
			Type: siterebuild.SinkMakers, ProjectID: cfg.Site.Makers.ProjectID,
			Region: cfg.Site.Makers.Region, TokenEnv: cfg.Site.Makers.TokenEnv,
		}}
	}
	return &Server{cfg: cfg}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/-/health", s.handleHealth)
	mux.HandleFunc("/-/version", s.handleVersion)
	mux.HandleFunc("/-/site", s.handleSiteStatus)
	mux.HandleFunc(publishproto.AgentPreflightPath, s.handlePublishPreflight)
	mux.HandleFunc("/v1/drop/", s.handleDrop)
	mux.HandleFunc("/v1/cas/credentials", s.handleCASCredentials)
	mux.HandleFunc("/v1/cas/complete", s.handleCASComplete)
	mux.HandleFunc("/v1/cas/abort", s.handleCASAbort)
	mux.HandleFunc("/v1/staged/", s.handleStaged)
	mux.HandleFunc("/v1/runtime-profile", s.handleRuntimeProfile)
	mux.HandleFunc("/v1/publish", s.handlePublish)
	mux.HandleFunc("/v1/unpublish", s.handleUnpublish)
	// Tool distribution is the one read face a host CI reaches before it has
	// any relkit code: the file plus its SSH signature, both rooted in the
	// project's pinned public key (requirements 12.1, 12.2, 12.3).
	mux.HandleFunc(toolsPrefix, s.handleTools)
	mux.HandleFunc("/v1/tools", s.handleToolPublish)
	// The Catalog names the tool version the server serves right now. A tool
	// reads it after starting, on the same origin as the tool bytes.
	mux.HandleFunc("/v1/catalog", s.handleCatalog)
	// The management surface owns users, roles, products and tokens. The panel
	// writes through here; it never touches the agent's storage (requirement 13.6).
	mux.HandleFunc(adminPrefix, s.handleAdmin)
	mux.HandleFunc(strings.TrimSuffix(adminPrefix, "/"), s.handleAdmin)
	return mux
}

func (s *Server) handleSiteStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]any{"configured": false, "tokenPresent": false}
	sinks := s.cfg.Site.Sinks
	status["sinks"] = len(sinks)
	if len(sinks) > 0 {
		status["configured"] = true
	}
	// The deploy-event snapshot (site/status.json) is the panel-facing truth
	// about what the last rebuild actually deployed. Missing means no rebuild
	// has deployed since the feature landed; the panel shows that as "none".
	if snap, ok := siterebuild.ReadStatus(s.cfg.StateDir); ok {
		status["lastDeploy"] = snap
	}
	// Legacy fields stay populated from the first makers sink so older
	// operators/scripts reading projectId/tokenEnv keep working.
	for _, spec := range sinks {
		if spec.Type != siterebuild.SinkMakers {
			continue
		}
		tokenEnv := spec.TokenEnv
		if tokenEnv == "" {
			tokenEnv = makers.DefaultTokenEnv
		}
		status["projectId"] = spec.ProjectID
		status["region"] = spec.Region
		status["tokenEnv"] = tokenEnv
		status["tokenPresent"] = strings.TrimSpace(os.Getenv(tokenEnv)) != ""
		break
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) productLock(product string) *sync.Mutex {
	v, _ := s.locks.LoadOrStore(product, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func (s *Server) rebuildSite(printer func(string)) (bool, error) {
	s.siteMu.Lock()
	defer s.siteMu.Unlock()
	registered := s.productIDs()
	products := make([]siterebuild.Product, 0, len(registered))
	for id, product := range registered {
		products = append(products, siterebuild.Product{
			ID: id, Root: product.Root, Profile: product.Profile,
		})
	}
	return siterebuild.Rebuild(siterebuild.Config{
		Sinks: s.cfg.Site.Sinks, StateDir: s.cfg.StateDir, AllowEmpty: s.allowEmptySite,
	}, products, printer)
}

// triggerSiteRebuild coalesces concurrent publishes while guaranteeing one
// more pass when a publish completes during an active rebuild.
func (s *Server) triggerSiteRebuild(product, version string) {
	s.siteTriggerMu.Lock()
	s.sitePending = true
	if s.siteRunning {
		s.siteTriggerMu.Unlock()
		return
	}
	s.siteRunning = true
	s.siteTriggerMu.Unlock()

	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("site rebuild after %s/%s panicked: %v", product, version, recovered)
				s.siteTriggerMu.Lock()
				restart := s.sitePending
				s.siteRunning = false
				s.siteTriggerMu.Unlock()
				if restart {
					s.triggerSiteRebuild(product, version)
				}
			}
		}()
		for {
			s.siteTriggerMu.Lock()
			if !s.sitePending {
				s.siteRunning = false
				s.siteTriggerMu.Unlock()
				return
			}
			s.sitePending = false
			s.siteTriggerMu.Unlock()

			if _, err := s.rebuildSite(func(line string) { log.Printf("%s", line) }); err != nil {
				log.Printf("site rebuild after %s/%s: %v", product, version, err)
			}
		}
	}()
}

// credentials returns the live token hashes. The management surface replaces
// this slice when a token is issued or revoked, so callers take a snapshot.
func (s *Server) credentials() []agentadmin.Credential {
	s.productMu.RLock()
	defer s.productMu.RUnlock()
	return append([]agentadmin.Credential(nil), s.cfg.credentials...)
}

func (s *Server) lookupCredential(r *http.Request) *agentadmin.Credential {
	header := r.Header.Get("Authorization")
	value, found := strings.CutPrefix(header, "Bearer ")
	if !found {
		return nil
	}
	presented := hashToken(strings.TrimSpace(value))
	credentials := s.credentials()
	var matched *agentadmin.Credential
	for i := range credentials {
		if subtle.ConstantTimeCompare(presented, credentials[i].Hash) == 1 {
			matched = &credentials[i]
		}
	}
	return matched
}

func (s *Server) requireAuthFor(w http.ResponseWriter, r *http.Request, product string) bool {
	if len(s.credentials()) == 0 {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "uploads are disabled on this server: no product token has been issued yet", http.StatusMethodNotAllowed)
		return false
	}
	cred := s.lookupCredential(r)
	if cred == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-agent"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	// A token is scoped to exactly one product, so an operator who reaches for the
	// wrong product gets a message that names both sides instead of a bare
	// forbidden (requirement 11.1).
	if !cred.Allows(product) {
		http.Error(w, fmt.Sprintf("forbidden: this token authorizes product %q, not %q", cred.Product, product), http.StatusForbidden)
		return false
	}
	// Every publisher endpoint funnels through here, so the protocol handshake
	// is checked in one place and cannot be forgotten on a new route.
	return s.requirePublishProtocol(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, publishproto.Identity(version))
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, publishproto.Identity(version))
}

func (s *Server) handleStaged(w http.ResponseWriter, r *http.Request) {
	product, versionRaw, rest, ok := parseStagedPath(r.URL.Path)
	if !ok {
		http.Error(w, "expected /v1/staged/{product}/{version}", http.StatusBadRequest)
		return
	}
	if !s.requireAuthFor(w, r, product) {
		return
	}
	version, ok := cleanVersion(versionRaw)
	if !ok {
		http.Error(w, "invalid version", http.StatusBadRequest)
		return
	}
	if _, ok := s.product(product); !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return
	}
	if len(rest) == 0 {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.putStagedWhole(w, r, product, version)
		return
	}
	s.handleStagedUpload(w, r, product, version, rest)
}

func (s *Server) putStagedWhole(w http.ResponseWriter, r *http.Request, product, version string) {
	body := http.MaxBytesReader(w, r.Body, s.cfg.MaxUpload)
	tmp, err := os.CreateTemp("", "relkit-staged-*.tar.gz")
	if err != nil {
		http.Error(w, "temp file", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), body)
	closeErr := tmp.Close()
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	if closeErr != nil {
		http.Error(w, "temp close failed", http.StatusInternalServerError)
		return
	}
	sum := hex.EncodeToString(h.Sum(nil))
	dest, err := s.installStagedArchive(product, version, tmpPath, sum)
	if err != nil {
		writeStagedErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"product": product,
		"version": version,
		"bytes":   n,
		"sha256":  sum,
		"path":    dest,
	})
}

type stagedHTTPError struct {
	status int
	msg    string
}

func (e *stagedHTTPError) Error() string { return e.msg }

func writeStagedErr(w http.ResponseWriter, err error) {
	if se, ok := err.(*stagedHTTPError); ok {
		http.Error(w, se.msg, se.status)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func (s *Server) installStagedArchive(product, version, tmpPath, sum string) (string, error) {
	pc, ok := s.product(product)
	if !ok {
		return "", &stagedHTTPError{http.StatusNotFound, "unknown product"}
	}
	mu := s.productLock(product)
	if !mu.TryLock() {
		return "", &stagedHTTPError{http.StatusConflict, "publish or staged upload already in progress for this product"}
	}
	defer mu.Unlock()

	dest := stage.StagingDir(pc.Root, version)
	_ = os.RemoveAll(dest)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", &stagedHTTPError{http.StatusInternalServerError, "mkdir staged"}
	}
	if err := extractTarGz(tmpPath, dest, s.cfg.MaxFiles); err != nil {
		_ = os.RemoveAll(dest)
		log.Printf("staged extract %s/%s: %v", product, version, err)
		return "", &stagedHTTPError{http.StatusBadRequest, "extract: " + err.Error()}
	}
	stagedDocument, err := stage.LoadStaged(pc.Root, version)
	if err != nil {
		_ = os.RemoveAll(dest)
		log.Printf("staged tree %s/%s: %v", product, version, err)
		return "", &stagedHTTPError{http.StatusBadRequest, "invalid staged tree: " + err.Error()}
	}
	// ADR 0019 §1 retired the release-policy.json control plane. A staged tree
	// carries its project declaration and ArtifactSet; validateVNextStaged
	// proves they describe this product's staged release.
	if err := s.validateVNextStaged(product, version, pc, stagedDocument); err != nil {
		_ = os.RemoveAll(dest)
		return "", &stagedHTTPError{http.StatusBadRequest, "invalid vNext staged tree: " + err.Error()}
	}
	if err := s.writeStagedSHA(product, version, sum); err != nil {
		_ = os.RemoveAll(dest)
		return "", &stagedHTTPError{http.StatusInternalServerError, "persist staged sha256: " + err.Error()}
	}
	return dest, nil
}

// validateVNextStaged proves a staged tree is what the server itself would
// publish: the project declaration names this product, and the ArtifactSet
// describes exactly the staged version, channel, and release code.
//
// There is no resolution, no signed input bundle, and no control-plane
// watermark any more. The server has one root, so provenance is "this server
// accepted this tree", not "some earlier signed snapshot authorised it"
// (requirements 10.3, 14.4).
func (s *Server) validateVNextStaged(
	product string,
	version string,
	pc ProductConfig,
	stagedDocument *model.StagedDocument,
) error {
	if s.cfg.Root == nil {
		return fmt.Errorf("agent has no server root: configure rootKeyId and custodyManifest so staged releases can be verified")
	}
	root := stage.StagingDir(pc.Root, version)
	productDocument, err := vnextproject.Load(filepath.Join(root, vnextrelease.ProjectFile))
	if err != nil {
		return err
	}
	if productDocument.Product != product {
		return fmt.Errorf("project document declares product %q, route is %q", productDocument.Product, product)
	}
	// The staged project must trust this same server: a host tree built against
	// another server cannot be published here, because every document this
	// server emits is signed with its one root.
	if err := productDocumentRootMatches(productDocument, s.cfg.Root); err != nil {
		return err
	}
	if stagedDocument.Product != product || stagedDocument.Version != version {
		return fmt.Errorf("staged product/version does not match route")
	}
	if len(productDocument.Channels) != 0 && !slices.Contains(productDocument.Channels, stagedDocument.Channel) {
		return fmt.Errorf("project does not declare channel %q", stagedDocument.Channel)
	}
	artifactDocument, err := artifactset.Load(filepath.Join(root, vnextrelease.ArtifactSetFile))
	if err != nil {
		return err
	}
	if artifactDocument.Product != product ||
		artifactDocument.Version != version ||
		artifactDocument.Channel != stagedDocument.Channel {
		return fmt.Errorf("ArtifactSet identity does not match staged release")
	}
	if int64(artifactDocument.ReleaseCode) != stagedDocument.Code {
		return fmt.Errorf(
			"ArtifactSet releaseCode %d does not match staged release code %d",
			artifactDocument.ReleaseCode,
			stagedDocument.Code,
		)
	}
	return nil
}

// productDocumentRootMatches checks that the staged project trusts the root
// this server signs with. The host writes that public key verbatim from what
// the server handed out at product registration, so a mismatch means the tree
// was built for a different deployment.
func productDocumentRootMatches(document *vnextproject.Document, root *config.RootKey) error {
	publicKey, err := document.RootKey()
	if err != nil {
		return err
	}
	if len(root.PublicKey) == 0 {
		return fmt.Errorf("agent server root has no public key")
	}
	if !publicKey.Equal(root.PublicKey) {
		return fmt.Errorf("project rootPublicKey does not match this server's root")
	}
	return nil
}


// parseStagedPath accepts /v1/staged/{product}/{version}[/...], including a
// doubled slash when RELKIT_PUBLISH_URL was configured with a trailing slash.
func parseStagedPath(urlPath string) (product, version string, rest []string, ok bool) {
	cleaned := path.Clean("/" + strings.TrimSpace(urlPath))
	restPath := strings.TrimPrefix(cleaned, "/v1/staged/")
	if restPath == cleaned {
		return "", "", nil, false
	}
	parts := strings.Split(strings.Trim(restPath, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", nil, false
	}
	if len(parts) == 2 {
		return parts[0], parts[1], nil, true
	}
	return parts[0], parts[1], parts[2:], true
}

// parseStagedRoute accepts /v1/staged/{product}/{version} with no extra segments.
func parseStagedRoute(urlPath string) (product, version string, ok bool) {
	p, v, rest, ok := parseStagedPath(urlPath)
	return p, v, ok && len(rest) == 0
}

func (s *Server) stagedSHAPath(product, version string) string {
	return filepath.Join(s.cfg.StateDir, "staged", product, version+".sha256")
}

func (s *Server) writeStagedSHA(product, version, sum string) error {
	path := s.stagedSHAPath(product, version)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sha256-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.WriteString(tmp, strings.ToLower(sum)+"\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Remove(path) // Windows cannot rename over an existing file.
	return os.Rename(tmpPath, path)
}

func (s *Server) readStagedSHA(product, version string) (string, error) {
	data, err := os.ReadFile(s.stagedSHAPath(product, version))
	if err != nil {
		return "", err
	}
	sum := strings.TrimSpace(strings.ToLower(string(data)))
	if !validSHA256Hex(sum) {
		return "", fmt.Errorf("invalid stored sha256")
	}
	return sum, nil
}

func validSHA256Hex(sum string) bool {
	if len(sum) != sha256.Size*2 {
		return false
	}
	for i := 0; i < len(sum); i++ {
		c := sum[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func sha256HexEqual(presented, stored string) bool {
	a := strings.ToLower(strings.TrimSpace(presented))
	b := strings.ToLower(strings.TrimSpace(stored))
	if !validSHA256Hex(a) || !validSHA256Hex(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func loadProductConfig(pc ProductConfig, version string, root *config.RootKey) (*config.Config, string, error) {
	productDocument, err := vnextproject.Load(filepath.Join(stage.StagingDir(pc.Root, version), vnextrelease.ProjectFile))
	if err != nil {
		return nil, "", err
	}
	profile, err := config.LoadPublishProfile(pc.Profile)
	if err != nil {
		return nil, "", fmt.Errorf("profile %s: %w", pc.Profile, err)
	}
	cfg, err := mergeVNextPublishConfig(productDocument, profile, pc.Root, root)
	if err != nil {
		return nil, "", err
	}
	return cfg, "vNext project bundle + " + pc.Profile, nil
}

func mergeVNextPublishConfig(
	productDocument *vnextproject.Document,
	profile *config.PublishProfile,
	productRoot string,
	root *config.RootKey,
) (*config.Config, error) {
	if productDocument == nil || profile == nil {
		return nil, fmt.Errorf("vNext project and publish profile are required")
	}
	if productDocument.Product != profile.Product {
		return nil, fmt.Errorf("publish profile product %q does not match project product %q", profile.Product, productDocument.Product)
	}
	if len(productDocument.Channels) == 0 {
		return nil, fmt.Errorf("vNext publisher project must declare channels")
	}
	// One server, one root: every product this server publishes is signed by
	// the same identity, so signing is not part of the product declaration and
	// there is nothing per-product to assemble (requirement 10.5).
	if root == nil {
		return nil, fmt.Errorf("server root is required to publish: resolve the custody manifest root before merging the publish config")
	}
	cfg := &config.Config{
		Root:           productRoot,
		Product:        productDocument.Product,
		DefaultChannel: productDocument.Channels[0],
		Channels:       append([]string(nil), productDocument.Channels...),
		CodeStrategy:   "explicit",
		RootKeyIDValue: root.KeyID,
		Backends:       cloneBackendConfig(profile.Backends),
		PublishTo:      append([]string(nil), profile.PublishTo...),
	}
	cfg.SetRoot(root)
	// The vNext project document is the only site declaration a migrated
	// product has: relkit.json is gone. Without this the publisher sees an
	// empty SiteConfig, hasSiteCopy is always false, and site/<product>.json
	// silently stops being written even though the product still appears in
	// every protocol document. That is the 2026-10 site outage.
	if productDocument.Site != nil {
		cfg.Site = config.SiteConfig{
			Title:       productDocument.Site.Title,
			Description: productDocument.Site.Description,
			Homepage:    productDocument.Site.Homepage,
		}
	}
	if profile.Directory != nil {
		cfg.Directory = &config.DirectoryConfig{
			PublishTo: append([]string(nil), profile.Directory.PublishTo...),
			EntryURLs: append([]string(nil), profile.Directory.EntryURLs...),
			Services:  append([]config.DirectoryServiceConfig(nil), profile.Directory.Services...),
		}
	}
	if err := completeVNextDirectory(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// completeVNextDirectory keeps deployment URLs out of relkit.project.json.
// Existing publish profiles only had to name the backends that receive the
// signed directory; derive the standard consumer URLs from those backend
// base URLs when the profile does not override them explicitly.
func completeVNextDirectory(cfg *config.Config) error {
	if cfg.Directory == nil {
		return fmt.Errorf("vNext publish profile directory is required")
	}
	targets := append([]string(nil), cfg.Directory.PublishTo...)
	if len(targets) == 0 {
		targets = append(targets, cfg.PublishTo...)
	}
	if len(targets) == 0 {
		return fmt.Errorf("vNext publish profile has no directory publication backends")
	}
	type endpoint struct {
		name string
		base string
	}
	endpoints := make([]endpoint, 0, len(targets))
	seenBase := map[string]bool{}
	for _, name := range targets {
		values, ok := cfg.Backends[name]
		if !ok {
			return fmt.Errorf("directory backend %q is not configured", name)
		}
		raw, _ := values["baseUrl"].(string)
		base, err := normalizedConsumerBaseURL(raw)
		if err != nil {
			return fmt.Errorf("directory backend %q baseUrl: %w", name, err)
		}
		if seenBase[base] {
			continue
		}
		seenBase[base] = true
		endpoints = append(endpoints, endpoint{name: name, base: base})
	}
	if len(cfg.Directory.EntryURLs) == 0 {
		for _, item := range endpoints {
			cfg.Directory.EntryURLs = append(
				cfg.Directory.EntryURLs,
				item.base+"directory/"+url.PathEscape(cfg.Product)+".pb",
			)
		}
	}
	if len(cfg.Directory.Services) == 0 {
		for endpointIndex, item := range endpoints {
			for _, channel := range cfg.Channels {
				id := item.name + "-" + channel
				cfg.Directory.Services = append(cfg.Directory.Services, config.DirectoryServiceConfig{
					ID:       id,
					Priority: int32(endpointIndex + 1),
					IndexURL: item.base + "index/" + url.PathEscape(cfg.Product) + "/" + url.PathEscape(channel) + ".pb",
					Channel:  channel,
				})
			}
		}
	}
	return nil
}

func normalizedConsumerBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("must be an absolute http(s) URL, got %q", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("must be http(s), got %q", raw)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("must not contain query or fragment, got %q", raw)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/"
	return parsed.String(), nil
}

func cloneBackendConfig(source map[string]map[string]any) map[string]map[string]any {
	result := make(map[string]map[string]any, len(source))
	for name, values := range source {
		cloned := make(map[string]any, len(values))
		for key, value := range values {
			cloned[key] = value
		}
		result[name] = cloned
	}
	return result
}

type runtimeProfileRequest struct {
	Product string                `json:"product"`
	Project vnextproject.Document `json:"project"`
	To      []string              `json:"to,omitempty"`
	DryRun  bool                  `json:"dryRun,omitempty"`
}

// handleRuntimeProfile bootstraps a signed Runtime Profile before a
// self-subscribing product can create its first vNext resolution. The product
// private key remains on the agent host; the caller receives only the signed
// public document. The product lock is shared with normal publication so the
// directory sequence cannot race a release.
func (s *Server) handleRuntimeProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var req runtimeProfileRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Product == "" || req.Project.Product == "" {
		http.Error(w, "product and project are required", http.StatusBadRequest)
		return
	}
	if req.Project.Product != req.Product {
		http.Error(w, "project product does not match request product", http.StatusBadRequest)
		return
	}
	if err := req.Project.Validate(); err != nil {
		http.Error(w, "invalid project: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !s.requireAuthFor(w, r, req.Product) {
		return
	}
	pc, ok := s.product(req.Product)
	if !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return
	}
	mu := s.productLock(req.Product)
	if !mu.TryLock() {
		http.Error(w, "publish already in progress for this product", http.StatusConflict)
		return
	}
	defer mu.Unlock()

	profile, err := config.LoadPublishProfile(pc.Profile)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"ok": false, "code": "runtime_profile.publish_profile_invalid", "error": err.Error(),
		})
		return
	}
	cfg, err := mergeVNextPublishConfig(&req.Project, profile, pc.Root, s.cfg.Root)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"ok": false, "code": "runtime_profile.config_invalid", "error": err.Error(),
		})
		return
	}
	var logs []string
	printer := func(line string) { logs = append(logs, line) }
	preview, err := directory.Set(cfg, directory.Options{To: req.To, DryRun: true}, printer)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"ok": false, "code": "runtime_profile.directory_preflight_failed", "error": err.Error(), "log": logs,
		})
		return
	}
	result, err := vnextruntimeprofile.Publish(cfg, req.Project, vnextruntimeprofile.PublishOptions{
		To:             req.To,
		ProfileVersion: preview.DirectorySequence,
		DryRun:         req.DryRun,
		Signer:         envelope.Signer{KeyID: s.cfg.Root.KeyID, Seed: s.cfg.Root.Seed},
	}, printer)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"ok": false, "code": "runtime_profile.publish_failed", "error": err.Error(), "log": logs,
		})
		return
	}
	directorySequence := preview.DirectorySequence
	if !req.DryRun {
		publishedDirectory, directoryErr := directory.Set(cfg, directory.Options{
			To: req.To, RuntimeProfile: &result.Reference,
		}, printer)
		if directoryErr != nil {
			writeJSON(w, http.StatusConflict, map[string]any{
				"ok": false, "code": "runtime_profile.directory_commit_failed", "error": directoryErr.Error(),
				"profile": result.Profile, "reference": result.Reference, "log": logs,
			})
			return
		}
		directorySequence = publishedDirectory.DirectorySequence
		s.triggerSiteRebuild(req.Product, "runtime-profile")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "dryRun": req.DryRun, "profile": result.Profile,
		"reference": result.Reference, "directorySequence": directorySequence, "log": logs,
	})
}

type publishRequest struct {
	Product        string   `json:"product"`
	Version        string   `json:"version"`
	To             []string `json:"to"`
	DryRun         bool     `json:"dryRun"`
	AllowBackfill  bool     `json:"allowBackfill"`
	AllowPartial   bool     `json:"allowPartial"`
	IdempotencyKey string   `json:"idempotencyKey"`
	StagedSHA256   string   `json:"stagedSha256"`
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var req publishRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	version, ok := cleanVersion(req.Version)
	if !ok || req.Product == "" {
		http.Error(w, "product and version required", http.StatusBadRequest)
		return
	}
	if req.AllowPartial {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":       false,
			"code":     "release.allow_partial_retired",
			"severity": "error",
			"stage":    "publish",
			"error":    "allowPartial is not permitted on the normal publish path; repair divergence explicitly",
		})
		return
	}
	if !s.requireAuthFor(w, r, req.Product) {
		return
	}
	pc, ok := s.product(req.Product)
	if !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return
	}
	if req.StagedSHA256 != "" {
		stored, err := s.readStagedSHA(req.Product, version)
		if err != nil {
			http.Error(w, "load staged sha256: "+err.Error(), http.StatusConflict)
			return
		}
		if !sha256HexEqual(req.StagedSHA256, stored) {
			http.Error(w, "staged sha256 mismatch; re-upload", http.StatusConflict)
			return
		}
	}

	idemKey := req.IdempotencyKey
	if idemKey == "" && req.StagedSHA256 != "" {
		idemKey = req.Product + "/" + version + "/" + req.StagedSHA256
	}
	if idemKey != "" {
		if cached, ok := s.loadIdempotent(idemKey); ok {
			writeJSON(w, http.StatusOK, cached)
			if !req.DryRun {
				s.triggerSiteRebuild(req.Product, version)
			}
			return
		}
	}

	mu := s.productLock(req.Product)
	if !mu.TryLock() {
		http.Error(w, "publish already in progress for this product", http.StatusConflict)
		return
	}
	defer mu.Unlock()

	cfg, source, err := loadProductConfig(pc, version, s.cfg.Root)
	if err != nil {
		http.Error(w, "load publish config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if cfg.Product != "" && cfg.Product != req.Product {
		http.Error(w, "product mismatch with publish config", http.StatusBadRequest)
		return
	}

	staged, err := stage.LoadStaged(cfg.Root, version)
	if err != nil {
		http.Error(w, "load staged: "+err.Error(), http.StatusBadRequest)
		return
	}
	if mismatches := stage.VerifyPresentStagedHashes(cfg, staged); len(mismatches) > 0 {
		http.Error(w, "staged hash mismatch; re-upload", http.StatusConflict)
		return
	}

	var logs []string
	logs = append(logs, "publish config: "+source)
	printer := func(line string) { logs = append(logs, line) }
	var runtimeProfileResult *vnextruntimeprofile.PublishResult
	if s.cfg.Root == nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"ok": false, "code": "release.root_unavailable",
			"error": "agent server root is not resolved; configure rootKeyId and custodyManifest",
			"log":   logs,
		})
		return
	}
	if !req.DryRun && cfg.Directory != nil {
		// The profile is signed by the server root and points at the directory
		// entries this release publishes to (requirements 10.4, 12.6). There is
		// no per-product runtime profile root and no opt-in flag: every release
		// carries one.
		productDocument, projectErr := vnextproject.Load(filepath.Join(stage.StagingDir(cfg.Root, version), vnextrelease.ProjectFile))
		if projectErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "code": "release.project_invalid", "error": projectErr.Error(), "log": logs})
			return
		}
		preview, previewErr := directory.Set(cfg, directory.Options{To: req.To, DryRun: true}, printer)
		if previewErr != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "code": "release.directory_preflight_failed", "error": previewErr.Error(), "log": logs})
			return
		}
		runtimeProfileResult, err = vnextruntimeprofile.Publish(cfg, *productDocument, vnextruntimeprofile.PublishOptions{
			To:             req.To,
			ProfileVersion: preview.DirectorySequence,
			Signer:         envelope.Signer{KeyID: s.cfg.Root.KeyID, Seed: s.cfg.Root.Seed},
		}, printer)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "code": "release.runtime_profile_failed", "error": err.Error(), "log": logs})
			return
		}
	}
	index, err := publish.Run(cfg, version, req.To, req.DryRun, req.AllowBackfill, req.AllowPartial, printer)
	if err != nil {
		response := map[string]any{"ok": false, "error": err.Error(), "log": logs}
		status := http.StatusBadRequest
		var publishErr publish.Error
		if errors.As(err, &publishErr) && publishErr.Code != "" {
			response["code"] = publishErr.Code
			if publishErr.Evidence != nil {
				response["evidence"] = publishErr.Evidence
			}
			if publishErr.Code == "release.partial_commit" {
				status = http.StatusConflict
			}
		}
		writeJSON(w, status, response)
		return
	}
	var directorySequence int64
	if !req.DryRun && cfg.Directory != nil {
		directoryOptions := directory.Options{To: req.To}
		if runtimeProfileResult != nil {
			directoryOptions.RuntimeProfile = &runtimeProfileResult.Reference
		}
		directoryDocument, dirErr := directory.Set(cfg, directoryOptions, printer)
		if dirErr != nil {
			logs = append(logs, "directory: "+dirErr.Error())
			resp := map[string]any{
				"ok":       false,
				"code":     "release.directory_partial",
				"severity": "error",
				"stage":    "publish",
				"error":    "signed index committed, but directory publication failed; fix forward and verify the released state",
				"message":  "signed index committed, but directory publication failed: " + dirErr.Error(),
				"cause":    dirErr.Error(),
				"product":  req.Product,
				"version":  version,
				"log":      logs,
			}
			if index != nil {
				resp["sequence"] = index.Sequence
				resp["channel"] = index.Channel
			}
			writeJSON(w, http.StatusConflict, resp)
			return
		}
		directorySequence = directoryDocument.DirectorySequence
	}
	if !req.DryRun {
		if cfg.Directory == nil {
			writeJSON(w, http.StatusConflict, map[string]any{
				"ok":       false,
				"code":     "release.directory_missing",
				"severity": "error",
				"stage":    "verify-released",
				"error":    "releases require a consumer-visible directory",
				"product":  req.Product,
				"version":  version,
				"log":      logs,
			})
			return
		}
		releasedOptions := verify.ReleasedOptions{
			Channel:                  staged.Channel,
			To:                       req.To,
			Version:                  version,
			MinimumIndexSequence:     index.Sequence,
			MinimumDirectorySequence: directorySequence,
		}
		if runtimeProfileResult != nil {
			releasedOptions.RuntimeProfileSHA256 = runtimeProfileResult.Reference.Sha256
			releasedOptions.RuntimeProfileDigest = runtimeProfileResult.Profile.Digest
		}
		findings, verifyErr := verify.RunReleased(cfg, releasedOptions, printer)
		if verifyErr != nil || !findings.OK() {
			cause := ""
			if verifyErr != nil {
				cause = verifyErr.Error()
			} else {
				cause = strings.Join(findings.Errors, "; ")
			}
			writeJSON(w, http.StatusConflict, map[string]any{
				"ok":       false,
				"code":     "release.verify_failed",
				"severity": "error",
				"stage":    "verify-released",
				"error":    "release commit could not be verified from consumer-visible backends; fix forward",
				"message":  "release commit could not be verified from consumer-visible backends: " + cause,
				"cause":    cause,
				"product":  req.Product,
				"version":  version,
				"log":      logs,
			})
			return
		}
	}
	resp := map[string]any{
		"ok":      true,
		"product": req.Product,
		"version": version,
		"dryRun":  req.DryRun,
		"log":     logs,
	}
	if index != nil {
		resp["sequence"] = index.Sequence
		resp["channel"] = index.Channel
	}
	if idemKey != "" && !req.DryRun {
		_ = s.saveIdempotent(idemKey, resp)
	}
	writeJSON(w, http.StatusOK, resp)
	if !req.DryRun {
		s.triggerSiteRebuild(req.Product, version)
	}
}

func (s *Server) idemPath(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.cfg.StateDir, "idempotency", hex.EncodeToString(sum[:])+".json")
}

func (s *Server) loadIdempotent(key string) (map[string]any, bool) {
	s.idemMu.Lock()
	defer s.idemMu.Unlock()
	data, err := os.ReadFile(s.idemPath(key))
	if err != nil {
		return nil, false
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, false
	}
	out["idempotentReplay"] = true
	return out, true
}

func (s *Server) saveIdempotent(key string, resp map[string]any) error {
	s.idemMu.Lock()
	defer s.idemMu.Unlock()
	path := s.idemPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
