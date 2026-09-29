package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/publishproto"
)

// Multi-platform drop aggregation, collected into the CLI (ADR 0017,
// 2026-09-29 decision): platform CI jobs push their artifacts plus a /2
// partial manifest into the agent drop (`ci upload`), and one aggregate
// job pulls every platform manifest back, materializes the referenced
// files at their declared paths, and publishes through the exact
// stage → simulate → fake → publish chain the single-platform products
// (dec, cronkit) already use (`ci release --from-drop`). The agent drop
// endpoints (PUT/GET/HEAD/DELETE /v1/drop/{product}/{version}/{filename})
// are unchanged; the Python .selectors text sidecars retire with this
// surface. The stored manifest is byte-for-byte the staging-form /2
// manifest the packScript wrote, so the aggregate side reuses the same
// loader and invariants instead of a second wire format.

var dropScopePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var dropPlatformPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// validateDropScope rejects empty or unsafe scopes before they reach a
// URL. The scope isolates one CI build's drop files so parallel pipelines
// never cross-contaminate a product/version drop. Dot-only tokens (. ..)
// are rejected outright: they survive the charset check but are path
// traversal building blocks.
func validateDropScope(scope string) error {
	if scope == "." || scope == ".." {
		return fmt.Errorf("invalid drop scope %q; expected a BK_CI_BUILD_ID-style [A-Za-z0-9._-]+ token", scope)
	}
	if !dropScopePattern.MatchString(scope) {
		return fmt.Errorf("invalid drop scope %q; expected a BK_CI_BUILD_ID-style [A-Za-z0-9._-]+ token", scope)
	}
	return nil
}

// validateDropPlatform keeps manifest filename suffixes safe.
func validateDropPlatform(platform string) error {
	if !dropPlatformPattern.MatchString(platform) {
		return fmt.Errorf("invalid drop platform %q; expected [a-z0-9-]+ (e.g. windows, macos)", platform)
	}
	return nil
}

// detectDropPlatform infers the platform from the build machine, mirroring
// the Python entry's detect_platform: platform jobs run on their own
// runners, so the host OS is the platform.
func detectDropPlatform() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return "windows", nil
	case "darwin":
		return "macos", nil
	default:
		return "", fmt.Errorf("cannot infer the drop platform from %s; pass --platform explicitly", runtime.GOOS)
	}
}

// scopedDropName prefixes a drop filename with the build scope.
func scopedDropName(scope, filename string) string {
	return scope + "--" + filename
}

// dropManifestName is the agreed drop name for one platform's partial
// manifest. Every platform job uploads the same name shape; the
// aggregator downloads them per platform by convention, so the agent
// needs no LIST capability.
func dropManifestName(scope, platform string) string {
	return scopedDropName(scope, "release-artifacts-"+platform+".json")
}

// validateDropFileName keeps drop names single-segment.
func validateDropFileName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid drop filename %q", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid drop filename %q: path separators are not allowed", name)
	}
	return nil
}

// installDropName is the transport name for a group's install artifact:
// the explicit manifest filename when present, else the path basename.
func installDropName(install ciArtifactRef) (string, error) {
	name := strings.TrimSpace(install.Filename)
	if name == "" {
		name = filepath.Base(filepath.ToSlash(install.Path))
	}
	if err := validateDropFileName(name); err != nil {
		return "", err
	}
	return name, nil
}

// payloadDropName is the transport name for a payload tree. Explicit
// filenames are mandatory on the drop path: payload paths are
// directories, so there is no meaningful basename to fall back to, and
// the filename doubles as the archive name clients see in the index.
func payloadDropName(payload ciPayloadRef) (string, error) {
	name := strings.TrimSpace(payload.Filename)
	if name == "" {
		return "", fmt.Errorf("payload %s has no filename; drop transport requires an explicit payload filename", payload.Path)
	}
	if err := validateDropFileName(name); err != nil {
		return "", err
	}
	return name, nil
}

// readJSONMapFile loads a JSON object from disk.
func readJSONMapFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %v", filepath.ToSlash(path), err)
	}
	return config, nil
}

// readProductID reads relkit.json's product id (the drop URL namespace).
func readProductID(root string) (string, error) {
	config, err := readJSONMapFile(filepath.Join(root, "relkit.json"))
	if err != nil {
		return "", fmt.Errorf("relkit.json not found; run keys gen --execute first")
	}
	product, _ := config["product"].(string)
	if strings.TrimSpace(product) == "" {
		return "", fmt.Errorf("relkit.json must declare product for drop aggregation")
	}
	return strings.TrimSpace(product), nil
}

// agentBaseForDrop resolves the agent base URL the same way the publish
// path does: RELKIT_AGENT_URL beats relkit.json agent.url.
func agentBaseForDrop(root string) string {
	if url := strings.TrimSpace(os.Getenv("RELKIT_AGENT_URL")); url != "" {
		return url
	}
	config, err := readJSONMapFile(filepath.Join(root, "relkit.json"))
	if err != nil {
		return ""
	}
	agent, _ := config["agent"].(map[string]any)
	url, _ := agent["url"].(string)
	return strings.TrimSpace(url)
}

// dropClient is a minimal authenticated client for the agent drop plane.
// The base accepts both the site-root and /v1 forms (same normalization
// as the publish path, which fixed the POST /publish 405 class of bug).
type dropClient struct {
	baseURL string // always ends with /v1
	token   string
	product string
	version string
	client  *http.Client
}

// newDropClient validates the scope and resolves the agent base.
func newDropClient(root, product, version, scope, token string) (*dropClient, error) {
	if err := validateDropScope(scope); err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("RELKIT_UPLOAD_TOKEN is required for drop aggregation (CI injects the product agent Bearer)")
	}
	url := agentBaseForDrop(root)
	if url == "" {
		return nil, fmt.Errorf("relkit.json agent.url (or RELKIT_AGENT_URL) is required for drop aggregation")
	}
	base := strings.TrimRight(url, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return &dropClient{
		baseURL: base,
		token:   strings.TrimSpace(token),
		product: product,
		version: version,
		client:  &http.Client{Timeout: 600 * time.Second},
	}, nil
}

// dropURL builds /v1/drop/{product}/{version}/{filename}. The version
// keeps '+' literal: the agent accepts both '+' and %2B, and escaping
// would change the key the agent stores under.
func (c *dropClient) dropURL(filename string) string {
	return fmt.Sprintf("%s/drop/%s/%s/%s", c.baseURL, c.product, c.version, filename)
}

func (c *dropClient) send(request *http.Request) (*http.Response, error) {
	// Every publisher endpoint on the agent funnels through the same
	// publishproto handshake (requireAuthFor); a request without the
	// protocol headers parses as the degraded [0,0] offer and is rejected
	// with 426 publisher_upgrade_required before the handler runs.
	publishproto.Apply(request.Header)
	request.Header.Set("Authorization", "Bearer "+c.token)
	return c.client.Do(request)
}

// put uploads one local file into the drop.
func (c *dropClient) put(filename, sourcePath string) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPut, c.dropURL(filename), file)
	if err != nil {
		return err
	}
	request.ContentLength = info.Size()
	switch strings.ToLower(filepath.Ext(sourcePath)) {
	case ".json":
		request.Header.Set("Content-Type", "application/json")
	case ".zip":
		request.Header.Set("Content-Type", "application/zip")
	default:
		request.Header.Set("Content-Type", "application/octet-stream")
	}
	fmt.Printf("PUT %s (%d bytes)\n", c.dropURL(filename), info.Size())
	response, err := c.send(request)
	if err != nil {
		return fmt.Errorf("drop put failed: %s: %v", c.dropURL(filename), err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return fmt.Errorf("drop put failed: HTTP %d %s: %s", response.StatusCode, c.dropURL(filename), strings.TrimSpace(string(body)))
	}
	fmt.Printf("drop put ok: %s (HTTP %d)\n", filename, response.StatusCode)
	return nil
}

// get downloads one drop file to destPath (atomic temp + rename).
func (c *dropClient) get(filename, destPath string) error {
	request, err := http.NewRequest(http.MethodGet, c.dropURL(filename), nil)
	if err != nil {
		return err
	}
	fmt.Printf("GET %s\n", c.dropURL(filename))
	response, err := c.send(request)
	if err != nil {
		return fmt.Errorf("drop get failed: %s: %v", c.dropURL(filename), err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return fmt.Errorf("drop file missing: %s (HTTP 404)", filename)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return fmt.Errorf("drop get failed: HTTP %d %s: %s", response.StatusCode, c.dropURL(filename), strings.TrimSpace(string(body)))
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(destPath), ".drop-get-*")
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(temp, response.Body)
	closeErr := temp.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(temp.Name())
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	return os.Rename(temp.Name(), destPath)
}

// delete removes one drop file (post-publish cleanup; failures are
// warnings, never fatal).
func (c *dropClient) delete(filename string) error {
	request, err := http.NewRequest(http.MethodDelete, c.dropURL(filename), nil)
	if err != nil {
		return err
	}
	response, err := c.send(request)
	if err != nil {
		return fmt.Errorf("drop delete failed: %s: %v", c.dropURL(filename), err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return fmt.Errorf("drop delete failed: HTTP %d %s: %s", response.StatusCode, c.dropURL(filename), strings.TrimSpace(string(body)))
	}
	fmt.Printf("drop deleted: %s\n", filename)
	return nil
}

// zipDir writes a deterministic zip of a payload tree: entries sorted,
// forward-slash relpaths, deflate. The stage engine re-zips the
// materialized tree on the aggregate side, so only tree equality matters,
// not transport byte equality.
func zipDir(sourceDir, zipPath string) (int, error) {
	type treeEntry struct {
		relPath string
		absPath string
	}
	var entries []treeEntry
	err := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		entries = append(entries, treeEntry{relPath: filepath.ToSlash(rel), absPath: path})
		return nil
	})
	if err != nil {
		return 0, err
	}
	if len(entries) == 0 {
		return 0, fmt.Errorf("payload directory %s is empty", filepath.ToSlash(sourceDir))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].relPath < entries[j].relPath })
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o755); err != nil {
		return 0, err
	}
	file, err := os.Create(zipPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		source, err := os.Open(entry.absPath)
		if err != nil {
			return 0, err
		}
		target, err := writer.Create(entry.relPath)
		if err != nil {
			source.Close()
			return 0, err
		}
		_, copyErr := io.Copy(target, source)
		source.Close()
		if copyErr != nil {
			return 0, copyErr
		}
	}
	if err := writer.Close(); err != nil {
		return 0, err
	}
	if err := file.Close(); err != nil {
		return 0, err
	}
	return len(entries), nil
}

// unpackPayloadZip extracts a payload transport zip into destDir with the
// same security posture as the Python unpacker it replaces: absolute
// paths and .. traversal are rejected; nothing executes.
func unpackPayloadZip(archivePath, destDir string) (int, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, fmt.Errorf("open payload zip: %w", err)
	}
	defer reader.Close()
	written := 0
	for _, entry := range reader.File {
		name := strings.ReplaceAll(entry.Name, "\\", "/")
		for strings.HasPrefix(name, "./") {
			name = strings.TrimPrefix(name, "./")
		}
		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}
		if strings.HasPrefix(name, "/") {
			return 0, fmt.Errorf("%s contains an absolute path: %s", filepath.Base(archivePath), entry.Name)
		}
		traversal := false
		for _, part := range strings.Split(name, "/") {
			if part == ".." {
				traversal = true
				break
			}
		}
		if traversal {
			return 0, fmt.Errorf("%s escapes the payload root: %s", filepath.Base(archivePath), entry.Name)
		}
		target := filepath.Join(destDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return 0, err
		}
		source, err := entry.Open()
		if err != nil {
			return 0, err
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			source.Close()
			return 0, err
		}
		_, copyErr := io.Copy(file, source)
		source.Close()
		file.Close()
		if copyErr != nil {
			return 0, copyErr
		}
		written++
	}
	if written == 0 {
		return 0, fmt.Errorf("%s is an empty payload zip", filepath.Base(archivePath))
	}
	return written, nil
}

// selectorMapsEqual compares two selector maps.
func selectorMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if other, ok := b[key]; !ok || other != value {
			return false
		}
	}
	return true
}

// parseReleaseArtifactsDoc parses a /2 partial manifest from bytes:
// schema, version, and selector-group decoding run here; disk validation
// (validateGroups, archive existence) is deliberately deferred so the
// aggregate job can check every manifest before materializing any files.
func parseReleaseArtifactsDoc(data []byte, expectedVersion string) (*ciReleaseArtifacts, error) {
	var raw struct {
		Schema         string            `json:"schema"`
		Version        string            `json:"version"`
		SelectorGroups []json.RawMessage `json:"selectorGroups"`
		Archives       []ciArchiveRef    `json:"archives"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("release manifest is not readable JSON: %v", err)
	}
	if raw.Schema != "relkit.release-artifacts/2" {
		return nil, fmt.Errorf("release manifest schema must be 'relkit.release-artifacts/2'")
	}
	if strings.TrimSpace(raw.Version) != expectedVersion {
		return nil, fmt.Errorf("release manifest version %q does not match project version %q", raw.Version, expectedVersion)
	}
	if len(raw.SelectorGroups) == 0 {
		return nil, fmt.Errorf("selectorGroups must contain at least one group")
	}
	var groups []ciSelectorGroup
	for i, entry := range raw.SelectorGroups {
		group, err := decodeSelectorGroup(entry)
		if err != nil {
			return nil, fmt.Errorf("selectorGroups[%d]: %v", i, err)
		}
		groups = append(groups, group)
	}
	return &ciReleaseArtifacts{
		Schema:   raw.Schema,
		Version:  raw.Version,
		Groups:   groups,
		Archives: raw.Archives,
	}, nil
}

// selectorsKey renders a selector map for deterministic logs and errors.
func selectorsKey(selectors map[string]string) string {
	pairs := make([]string, 0, len(selectors))
	for key, value := range selectors {
		pairs = append(pairs, key+"="+value)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

// checkGroupOverlap rejects two groups sharing the same selectors: one
// version carries exactly one artifact set per selector profile, so a
// collision across platform manifests is a wiring bug, not a union.
func checkGroupOverlap(existing []ciSelectorGroup, group ciSelectorGroup) error {
	for _, other := range existing {
		if selectorMapsEqual(other.Selectors, group.Selectors) {
			return fmt.Errorf("duplicate selector profile [%s] across platform manifests; one version carries exactly one artifact set per profile", selectorsKey(group.Selectors))
		}
	}
	return nil
}

// uploadArtifacts pushes one platform's install artifacts, payload
// transport zips, and finally the manifest itself into the scoped drop.
// The manifest goes last on purpose: its presence is the aggregate job's
// signal that this platform job finished cleanly.
func uploadArtifacts(client *dropClient, root string, artifacts *ciReleaseArtifacts, manifestPath, scope, platform string) error {
	for i, group := range artifacts.Groups {
		installName, err := installDropName(group.Install)
		if err != nil {
			return fmt.Errorf("selectorGroups[%d].install: %v", i, err)
		}
		if err := client.put(scopedDropName(scope, installName), filepath.Join(root, filepath.FromSlash(group.Install.Path))); err != nil {
			return err
		}
		for j, payload := range group.Payloads {
			payloadName, err := payloadDropName(payload)
			if err != nil {
				return fmt.Errorf("selectorGroups[%d].payloads[%d]: %v", i, j, err)
			}
			tempZip := filepath.Join(os.TempDir(), fmt.Sprintf("relkit-drop-payload-%d.zip", time.Now().UnixNano()))
			count, err := zipDir(filepath.Join(root, filepath.FromSlash(payload.Path)), tempZip)
			if err != nil {
				_ = os.Remove(tempZip)
				return fmt.Errorf("selectorGroups[%d].payloads[%d]: %v", i, j, err)
			}
			fmt.Printf("payload transport zip: %s (%d files)\n", payloadName, count)
			uploadErr := client.put(scopedDropName(scope, payloadName), tempZip)
			_ = os.Remove(tempZip)
			if uploadErr != nil {
				return uploadErr
			}
		}
	}
	return client.put(dropManifestName(scope, platform), filepath.Join(root, filepath.FromSlash(manifestPath)))
}

// dropPlatforms resolves the platform list for from-drop aggregation:
// an explicit --platforms flag beats relkit.json release.dropPlatforms.
func dropPlatforms(root, explicit string) ([]string, error) {
	if strings.TrimSpace(explicit) != "" {
		var platforms []string
		for _, item := range strings.Split(explicit, ",") {
			platform := strings.TrimSpace(strings.ToLower(item))
			if platform == "" {
				continue
			}
			if err := validateDropPlatform(platform); err != nil {
				return nil, err
			}
			platforms = append(platforms, platform)
		}
		if len(platforms) == 0 {
			return nil, fmt.Errorf("--platforms must name at least one platform")
		}
		return platforms, nil
	}
	config, err := readJSONMapFile(filepath.Join(root, "relkit.json"))
	if err != nil {
		return nil, fmt.Errorf("relkit.json not found; run keys gen --execute first")
	}
	release, _ := config["release"].(map[string]any)
	rawList, _ := release["dropPlatforms"].([]any)
	var platforms []string
	for _, item := range rawList {
		platform, _ := item.(string)
		platform = strings.TrimSpace(strings.ToLower(platform))
		if platform == "" {
			continue
		}
		if err := validateDropPlatform(platform); err != nil {
			return nil, err
		}
		platforms = append(platforms, platform)
	}
	if len(platforms) == 0 {
		return nil, fmt.Errorf("from-drop aggregation needs a platform list; pass --platforms or declare release.dropPlatforms in relkit.json")
	}
	return platforms, nil
}

// assembleFromDrop pulls every platform's partial manifest out of the
// scoped drop, materializes the referenced install artifacts and payload
// trees at their declared paths under root, and returns the merged group
// list plus the drop filenames for post-publish cleanup. Disk validation
// runs after materialization, so the from-drop path shares the exact
// manifest invariants of the local pack path.
func assembleFromDrop(root, version, scope string, platforms []string) ([]ciSelectorGroup, []string, error) {
	product, err := readProductID(root)
	if err != nil {
		return nil, nil, err
	}
	client, err := newDropClient(root, product, version, scope, os.Getenv("RELKIT_UPLOAD_TOKEN"))
	if err != nil {
		return nil, nil, err
	}
	type platformManifest struct {
		platform  string
		artifacts *ciReleaseArtifacts
	}
	var collected []platformManifest
	var allGroups []ciSelectorGroup
	for _, platform := range platforms {
		if err := validateDropPlatform(platform); err != nil {
			return nil, nil, err
		}
		localManifest := filepath.Join(root, ".relkit", "drop-incoming", scope, "release-artifacts-"+platform+".json")
		if err := client.get(dropManifestName(scope, platform), localManifest); err != nil {
			return nil, nil, fmt.Errorf("platform %s: %v (was this platform job green?)", platform, err)
		}
		data, err := os.ReadFile(localManifest)
		if err != nil {
			return nil, nil, err
		}
		artifacts, err := parseReleaseArtifactsDoc(data, version)
		if err != nil {
			return nil, nil, fmt.Errorf("platform %s manifest: %v", platform, err)
		}
		for _, group := range artifacts.Groups {
			if err := checkGroupOverlap(allGroups, group); err != nil {
				return nil, nil, fmt.Errorf("platform %s: %v", platform, err)
			}
			allGroups = append(allGroups, group)
		}
		collected = append(collected, platformManifest{platform: platform, artifacts: artifacts})
	}
	// Materialize only after every manifest parsed and every group
	// checked: a bad manifest anywhere fails before a byte is staged.
	for _, pm := range collected {
		for i, group := range pm.artifacts.Groups {
			installName, err := installDropName(group.Install)
			if err != nil {
				return nil, nil, fmt.Errorf("platform %s selectorGroups[%d].install: %v", pm.platform, i, err)
			}
			dest := filepath.Join(root, filepath.FromSlash(group.Install.Path))
			if err := client.get(scopedDropName(scope, installName), dest); err != nil {
				return nil, nil, fmt.Errorf("platform %s: %v", pm.platform, err)
			}
			for j, payload := range group.Payloads {
				payloadName, err := payloadDropName(payload)
				if err != nil {
					return nil, nil, fmt.Errorf("platform %s selectorGroups[%d].payloads[%d]: %v", pm.platform, i, j, err)
				}
				zipDest := filepath.Join(root, ".relkit", "drop-incoming", scope, "payload", payloadName)
				if err := client.get(scopedDropName(scope, payloadName), zipDest); err != nil {
					return nil, nil, fmt.Errorf("platform %s: %v", pm.platform, err)
				}
				treeDest := filepath.Join(root, filepath.FromSlash(payload.Path))
				count, err := unpackPayloadZip(zipDest, treeDest)
				if err != nil {
					return nil, nil, fmt.Errorf("platform %s selectorGroups[%d].payloads[%d]: %v", pm.platform, i, j, err)
				}
				fmt.Printf("payload tree: %s (%d files)\n", filepath.ToSlash(payload.Path), count)
			}
		}
		if err := validateGroups(root, pm.artifacts); err != nil {
			return nil, nil, fmt.Errorf("platform %s manifest: %v", pm.platform, err)
		}
	}
	var cleanup []string
	for _, pm := range collected {
		cleanup = append(cleanup, dropManifestName(scope, pm.platform))
		for _, group := range pm.artifacts.Groups {
			if name, err := installDropName(group.Install); err == nil {
				cleanup = append(cleanup, scopedDropName(scope, name))
			}
			for _, payload := range group.Payloads {
				if name, err := payloadDropName(payload); err == nil {
					cleanup = append(cleanup, scopedDropName(scope, name))
				}
			}
		}
	}
	return allGroups, cleanup, nil
}

// cleanupDropFiles removes the build scope's drop files after a
// successful publish. Cleanup failures are warnings: the version is
// already published and the drop is scratch space.
func cleanupDropFiles(root, version, scope string, filenames []string) {
	product, err := readProductID(root)
	if err != nil {
		fmt.Printf("drop cleanup skipped: %v\n", err)
		return
	}
	client, err := newDropClient(root, product, version, scope, os.Getenv("RELKIT_UPLOAD_TOKEN"))
	if err != nil {
		fmt.Printf("drop cleanup skipped: %v\n", err)
		return
	}
	for _, filename := range filenames {
		if err := client.delete(filename); err != nil {
			fmt.Printf("drop cleanup warning: %v\n", err)
		}
	}
}
