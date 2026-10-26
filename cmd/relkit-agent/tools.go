package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/tools"
)

// Tool distribution is deliberately outside /v1/publish: host CI downloads
// relkit tooling before it has run any relkit code, so these endpoints are
// readable without a product token. That is safe because a tool file is
// useless unless its SSH signature verifies against the root public key the
// host project pins (requirement 12.2). Nothing here is product-scoped: every
// product on one server shares one relkit toolchain, which is what makes the
// CI template freeze-able (requirement 12.5).
const (
	toolsPrefix = "/v1/tools/"
	// toolSignatureSuffix marks a sidecar next to a tool file.
	toolSignatureSuffix = ".sig"
	// toolRootKeyFile publishes the server root public key so an operator can
	// paste it into a new project without reading the custody manifest.
	toolRootKeyFile = "root.pub"
	// toolCurrentAlias is the version segment host CI uses instead of a pinned
	// version number. The server resolves it against the signed Catalog, which
	// is the only place "which version is current" lives (requirement 12.6),
	// so the CI template never names a relkit version (requirement 12.5).
	//
	// It is read-only: an upload must name the numbered version it adds, or a
	// deploy pipeline could rewrite what every host is about to download.
	toolCurrentAlias = "current"
	// toolUploadLimit bounds one tool file. The updater and CLI are tens of
	// megabytes; a gigabyte cap leaves room without letting a bad upload fill
	// the state directory before the digest check runs.
	toolUploadLimit = 1 << 30
)

// toolStoreDir is where the deploy pipeline deposits released tool files.
func (s *Server) toolStoreDir() string {
	if strings.TrimSpace(s.cfg.ToolsDir) != "" {
		return s.cfg.ToolsDir
	}
	return filepath.Join(s.cfg.StateDir, "tools")
}

// handleTools serves /v1/tools/<version>/<file> and
// /v1/tools/<version>/<file>.sig. The signed manifest itself lives at
// /v1/tools/<version>/manifest.json so a host can fetch it and check every
// digest before executing anything.
//
// PUT is how the deploy pipeline deposits a file; it is gated by the deploy
// token because a tool file that a product token could overwrite is a supply
// chain hole. The manifest and its sidecars are never writable: they only come
// from signing.
func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, toolsPrefix)
	if rest == toolRootKeyFile {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// A new host copies this one line into its project declaration. It is
		// the same key the manifest carries, just easier to reach.
		rootPath := filepath.Join(s.toolStoreDir(), toolRootKeyFile)
		if info, err := os.Stat(rootPath); err != nil || info.IsDir() {
			http.Error(w, "no root public key published", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		http.ServeFile(w, r, rootPath)
		return
	}
	parts := strings.Split(rest, "/")
	// <version>/<file> names a file directly; <version>/<component>/<platform>
	// asks the server to resolve which file that is, and an extra "signature"
	// segment asks for its sidecar. The resolving form is what host CI uses: it
	// keeps file naming (and therefore every prefix rule) in the signed
	// manifest instead of in a frozen yaml.
	if len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != "" {
		s.handleToolByComponent(w, r, parts[0], parts[1], parts[2], false)
		return
	}
	if len(parts) == 4 && parts[3] == "signature" {
		s.handleToolByComponent(w, r, parts[0], parts[1], parts[2], true)
		return
	}
	// <version> alone is not a file; <version>/<file> is.
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	version, name := parts[0], parts[1]
	if !safeToolSegment(version) || !safeToolSegment(name) {
		http.Error(w, "invalid tool path", http.StatusBadRequest)
		return
	}
	viaCurrent := false
	if version == toolCurrentAlias {
		// The alias only reads. A deploy pipeline must name the numbered
		// version it publishes, otherwise it could silently repoint every host.
		if r.Method == http.MethodPut {
			http.Error(w, "current is a read alias; upload the numbered version", http.StatusBadRequest)
			return
		}
		resolved, err := s.currentToolVersion()
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		version = resolved
		viaCurrent = true
	}
	switch r.Method {
	case http.MethodPut:
		s.putTool(w, r, version, name)
		return
	case http.MethodGet, http.MethodHead:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	signature := strings.HasSuffix(name, toolSignatureSuffix)
	filePath := filepath.Join(s.toolStoreDir(), version, name)
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		http.Error(w, "no such tool", http.StatusNotFound)
		return
	}
	if signature {
		w.Header().Set("Content-Type", "application/pgp-signature; name=\""+name+"\"")
	} else if name == "manifest.json" {
		w.Header().Set("Content-Type", "application/json")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	// Tool bytes are immutable per version: a changed file is a new version,
	// so a long cache is correct and keeps CI downloads cheap. The `current`
	// alias is the exception: it points at a moving version, so caching it
	// would pin CI to a stale toolchain.
	if viaCurrent {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	if name != "manifest.json" {
		digest, err := fileSHA256Hex(filePath)
		if err == nil {
			w.Header().Set("ETag", `"`+digest+`"`)
		}
	}
	http.ServeFile(w, r, filePath)
}

// handleToolByComponent serves a tool by asking the signed manifest which file
// makes up a component on a platform.
//
// Host CI names "updater for windows-amd64", never a file name: the release can
// rename a binary without editing a frozen host yaml, and a host cannot
// accidentally fetch a file that is not part of the version it verified. Only
// GET/HEAD are allowed here; uploads always name the file they add.
func (s *Server) handleToolByComponent(w http.ResponseWriter, r *http.Request, version, component, platform string, signature bool) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !safeToolSegment(version) || !safeToolSegment(component) || !safeToolSegment(platform) {
		http.Error(w, "invalid tool path", http.StatusBadRequest)
		return
	}
	viaCurrent := false
	if version == toolCurrentAlias {
		resolved, err := s.currentToolVersion()
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		version = resolved
		viaCurrent = true
	}
	manifest, err := tools.Load(filepath.Join(s.toolStoreDir(), version, "manifest.json"))
	if err != nil {
		http.Error(w, "no manifest for "+version, http.StatusNotFound)
		return
	}
	row, ok := manifest.Component(component, platform)
	if !ok {
		http.Error(w, "version "+version+" has no "+component+" for "+platform, http.StatusNotFound)
		return
	}
	fileName := filepath.Base(row.Name)
	if signature {
		fileName += toolSignatureSuffix
	}
	filePath := filepath.Join(s.toolStoreDir(), version, fileName)
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		http.Error(w, "no such tool", http.StatusNotFound)
		return
	}
	if signature {
		w.Header().Set("Content-Type", "application/pgp-signature; name=\""+fileName+"\"")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		// X-Relkit-Tool-Name lets a template save the file under the name the
		// manifest signed, without parsing the manifest itself.
		w.Header().Set("X-Relkit-Tool-Name", row.Name)
		w.Header().Set("X-Relkit-Tool-SHA256", row.SHA256)
	}
	if viaCurrent {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.Header().Set("ETag", `"`+strings.TrimPrefix(row.SHA256, "sha256:")+`"`)
	http.ServeFile(w, r, filePath)
}

// putTool stores one uploaded tool file. It refuses the names signing owns so
// an upload can never bypass the root signature.
func (s *Server) putTool(w http.ResponseWriter, r *http.Request, version, name string) {
	defer r.Body.Close()
	if name == "manifest.json" || strings.HasSuffix(name, toolSignatureSuffix) {
		http.Error(w, name+" is produced by signing and cannot be uploaded", http.StatusBadRequest)
		return
	}
	if !s.requireToolPublishAuth(w, r) {
		return
	}
	dir := filepath.Join(s.toolStoreDir(), version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, "prepare tool directory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// A version is immutable once signed: re-uploading different bytes under
	// the same version would invalidate an already distributed signature.
	filePath := filepath.Join(dir, name)
	if _, err := os.Stat(filePath); err == nil {
		// Byte-identical re-upload is how a retried pipeline recovers, so
		// compare before refusing.
		existing, hashErr := fileSHA256Hex(filePath)
		if hashErr != nil {
			http.Error(w, "hash existing: "+hashErr.Error(), http.StatusInternalServerError)
			return
		}
		temp, err := os.CreateTemp(dir, ".upload-*.part")
		if err != nil {
			http.Error(w, "temp file", http.StatusInternalServerError)
			return
		}
		tempPath := temp.Name()
		defer os.Remove(tempPath)
		hash := sha256.New()
		if _, err := io.Copy(io.MultiWriter(temp, hash), http.MaxBytesReader(w, r.Body, toolUploadLimit)); err != nil {
			temp.Close()
			http.Error(w, "read body failed", http.StatusBadRequest)
			return
		}
		if err := temp.Close(); err != nil {
			http.Error(w, "temp close", http.StatusInternalServerError)
			return
		}
		if uploaded := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(uploaded, existing) {
			http.Error(w, fmt.Sprintf("tool %s for version %s already exists with a different digest; a changed file is a new version", name, version), http.StatusConflict)
			return
		}
		w.Header().Set("X-Relkit-Tool-Digest", existing)
		w.WriteHeader(http.StatusOK)
		return
	}
	// The staging file lives beside its destination on purpose. os.Rename is
	// atomic only within one filesystem, and the state directory is commonly a
	// separate mount (here /data, while the system temp dir is /tmp), so a
	// staging file in os.TempDir() fails with "invalid cross-device link". The
	// dot prefix keeps a half-written upload out of the served name space, and
	// the deferred remove clears it if this request dies partway.
	temp, err := os.CreateTemp(dir, ".upload-*.part")
	if err != nil {
		http.Error(w, "temp file", http.StatusInternalServerError)
		return
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temp, hash), http.MaxBytesReader(w, r.Body, toolUploadLimit))
	if err != nil {
		temp.Close()
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	if err := temp.Close(); err != nil {
		http.Error(w, "temp close", http.StatusInternalServerError)
		return
	}
	if written == 0 {
		http.Error(w, "empty tool file", http.StatusBadRequest)
		return
	}
	if err := os.Chmod(tempPath, 0o755); err != nil {
		http.Error(w, "chmod", http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tempPath, filePath); err != nil {
		http.Error(w, "install tool file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	w.Header().Set("X-Relkit-Tool-Digest", digest)
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok": true, "version": version, "name": name,
		"size": written, "sha256": "sha256:" + digest,
	})
}

// safeToolSegment rejects traversal and path separators in one URL segment.
func safeToolSegment(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	if strings.ContainsAny(value, `/\`) {
		return false
	}
	return !strings.Contains(value, "\x00")
}

// currentToolVersion resolves the `current` alias from the signed Catalog.
// The Catalog is the server's statement about which version it serves, so the
// alias needs no second copy of that fact and cannot disagree with it.
func (s *Server) currentToolVersion() (string, error) {
	document, err := catalog.Load(s.catalogPath())
	if err != nil {
		return "", fmt.Errorf("no current tool version: %v", err)
	}
	if strings.TrimSpace(document.Current) == "" {
		return "", fmt.Errorf("no current tool version: catalog names no release")
	}
	return document.Current, nil
}

func fileSHA256Hex(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// toolPublishRequest is the deploy pipeline's upload of one built tool set.
// The pipeline has already verified every file against the relkit Release
// assets; this endpoint re-checks the declared digest so a truncated upload
// can never be signed (requirement 12.6).
type toolPublishRequest struct {
	Version string            `json:"version"`
	Commit  string            `json:"commit"`
	Files   []toolPublishFile `json:"files"`
}

type toolPublishFile struct {
	// Component is the registry name of the tool (cli, updater, ops, ...). It
	// is optional only for backwards compatibility with an upload that names
	// files the registry way; the Catalog needs it to be right.
	Component string `json:"component,omitempty"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

// handleToolPublish implements POST /v1/tools. It is authorized by the deploy
// pipeline's own token (toolsTokenFile), never by a product upload token: the
// toolchain is shared by every product on the server, so letting a
// product-scoped token replace it would be a cross-product write.
func (s *Server) handleToolPublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	if !s.requireToolPublishAuth(w, r) {
		return
	}
	var req toolPublishRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	version := strings.TrimSpace(req.Version)
	if version == "" || len(req.Files) == 0 {
		http.Error(w, "version and files are required", http.StatusBadRequest)
		return
	}
	if !safeToolSegment(version) {
		http.Error(w, "invalid version", http.StatusBadRequest)
		return
	}
	// `current` is a read alias, not a version: publishing under it would make
	// the alias and the Catalog disagree about what is current.
	if version == toolCurrentAlias {
		http.Error(w, "current is a read alias; publish the numbered version", http.StatusBadRequest)
		return
	}
	if s.cfg.Root == nil {
		http.Error(w, "server root is not configured", http.StatusConflict)
		return
	}
	// The signed URLs name the same server hosts already talk to: the address
	// in their relkit.project.json, which is this agent's publicURL. Tools are
	// a CI concern, so they live on the CI face, not on the updater read plane.
	// A loopback origin is refused because no host could download from it,
	// and the publish would otherwise report success while signing dead links.
	baseURL := s.serverURL(r)
	if !isPublicOrigin(baseURL) {
		http.Error(w, fmt.Sprintf(
			"tool URLs need this server's public address, got %q; set publicURL in the agent config to the address hosts put in relkit.project.json",
			baseURL), http.StatusConflict)
		return
	}

	dir := filepath.Join(s.toolStoreDir(), version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, "prepare tool directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rows := make([]tools.Tool, 0, len(req.Files))
	for _, file := range req.Files {
		name := filepath.Base(strings.TrimSpace(file.Name))
		if name == "" || !safeToolSegment(name) {
			http.Error(w, "invalid file name", http.StatusBadRequest)
			return
		}
		filePath := filepath.Join(dir, name)
		info, err := os.Stat(filePath)
		if err != nil || info.IsDir() {
			http.Error(w, fmt.Sprintf("tool file %s is not on disk; upload it before publishing the manifest", name), http.StatusBadRequest)
			return
		}
		// The declared digest must match what is actually stored: the pipeline
		// verified the Release asset, and this check proves the byte-for-byte
		// copy arrived intact before the server signs it.
		actual, err := fileSHA256Hex(filePath)
		if err != nil {
			http.Error(w, "hash "+name+": "+err.Error(), http.StatusInternalServerError)
			return
		}
		want := strings.TrimPrefix(strings.TrimSpace(file.SHA256), "sha256:")
		if !strings.EqualFold(actual, want) {
			http.Error(w, fmt.Sprintf("tool file %s digest mismatch: disk %s, declared %s", name, actual, want), http.StatusBadRequest)
			return
		}
		payload, err := os.ReadFile(filePath)
		if err != nil {
			http.Error(w, "read "+name+": "+err.Error(), http.StatusInternalServerError)
			return
		}
		signature, err := tools.SSHSign(s.cfg.Root.Seed, payload)
		if err != nil {
			http.Error(w, "sign "+name+": "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(filePath+toolSignatureSuffix, []byte(tools.Armor(signature)), 0o644); err != nil {
			http.Error(w, "write "+name+toolSignatureSuffix+": "+err.Error(), http.StatusInternalServerError)
			return
		}
		component := strings.TrimSpace(file.Component)
		if component == "" {
			component = toolComponentFromFile(name)
		}
		// Platform is what hosts select on ("windows-amd64"), and the manifest
		// cannot be signed without it. The pipeline already states os and arch
		// separately, so it is derived here rather than demanded a third time:
		// a missing field that has exactly one correct value is a parsing job,
		// not an operator decision.
		platform := strings.TrimSpace(file.Platform)
		fileOS := strings.TrimSpace(file.OS)
		fileArch := strings.TrimSpace(file.Arch)
		if platform == "" && fileOS != "" && fileArch != "" {
			platform = fileOS + "-" + fileArch
		}
		rows = append(rows, tools.Tool{
			Component: component,
			Name:      name, Version: version, Platform: platform,
			OS: fileOS, Arch: fileArch, Size: info.Size(),
			SHA256:       tools.FileSHA256(sha256.Sum256(payload)),
			URL:          tools.ToolURL(baseURL, version, name),
			SignatureURL: tools.ToolURL(baseURL, version, name+toolSignatureSuffix),
		})
	}

	publicKey := s.cfg.Root.PublicKey
	encoded, err := tools.EncodePublicKey(publicKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fingerprint := s.cfg.Root.Fingerprint
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Platform != rows[j].Platform {
			return rows[i].Platform < rows[j].Platform
		}
		return rows[i].Name < rows[j].Name
	})
	manifest := tools.Document{
		Schema: tools.Schema, Version: version, Commit: req.Commit,
		RootKeyID: s.cfg.Root.KeyID, RootPublicKey: encoded, RootFingerprint: fingerprint,
		Tools: rows,
	}
	if err := manifest.Sign(s.cfg.Root.KeyID, s.cfg.Root.Seed); err != nil {
		http.Error(w, "sign manifest: "+err.Error(), http.StatusInternalServerError)
		return
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := writeToolManifest(manifestPath, manifest); err != nil {
		http.Error(w, "write manifest: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// The Catalog is signed from the manifest that was just signed, so "which
	// files make up this version" and "which version is current" can never
	// disagree (requirement 12.6).
	if err := s.writeCatalog(manifest); err != nil {
		http.Error(w, "write catalog: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// root.pub lets a new host copy the exact key to pin without reading the
	// custody manifest on the server.
	if err := os.WriteFile(filepath.Join(s.toolStoreDir(), "root.pub"), []byte(encoded+"\n"), 0o644); err != nil {
		http.Error(w, "write root.pub: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"version":  version,
		"manifest": tools.ToolURL(baseURL, version, "manifest.json"),
		"catalog":  baseURL + "/v1/catalog",
		"allowedSigner": tools.AllowedSignerLine(manifest.RootKeyID, publicKey),
		"tools":         len(rows),
	})
}

// requireToolPublishAuth checks the deploy pipeline token. Tool publishing has
// its own credential so a product token can never rewrite the shared toolchain.
func (s *Server) requireToolPublishAuth(w http.ResponseWriter, r *http.Request) bool {
	// Tools cannot be served without a root: an unsigned manifest is useless,
	// so reporting "not configured" is more useful than a bare 401.
	if s.cfg.Root == nil {
		http.Error(w, "server root is not configured", http.StatusConflict)
		return false
	}
	if len(s.cfg.ToolsTokenHash) == 0 {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "tool publishing is disabled on this server", http.StatusMethodNotAllowed)
		return false
	}
	value, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-agent"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	presented := hashToken(strings.TrimSpace(value))
	if subtle.ConstantTimeCompare(presented, s.cfg.ToolsTokenHash) != 1 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-agent"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func writeToolManifest(filePath string, manifest tools.Document) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, append(data, '\n'), 0o644)
}

// requestBaseURL reconstructs the externally visible origin. nginx terminates
// TLS in front of the agent, so the forwarded headers are the authority.
func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		if index := strings.Index(forwarded, ","); index >= 0 {
			forwarded = strings.TrimSpace(forwarded[:index])
		}
		scheme = forwarded
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if index := strings.Index(host, ","); index >= 0 {
		host = strings.TrimSpace(host[:index])
	}
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

// isPublicOrigin reports whether an absolute http(s) origin names a host other
// than this machine. Signed tool URLs are fetched by build machines, so an
// origin that only resolves on the server itself is never usable.
func isPublicOrigin(origin string) bool {
	parsed, err := neturl.Parse(strings.TrimSpace(origin))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	host := parsed.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		return false
	}
	return true
}
