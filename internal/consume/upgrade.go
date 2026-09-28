package consume

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/registry"
)

// PublishProtocolFallback mirrors PUBLISH_PROTOCOL_FALLBACK: releases that
// omit the protocol window fall back to 2/2.
const PublishProtocolFallback = 2

// UpdaterIpcFallback mirrors UPDATER_IPC_FALLBACK: releases that omit the
// updater IPC window fall back to 1/1.
const UpdaterIpcFallback = 1

// GitHubRepo is the release repository both artifact URL bases are derived
// from; RELKIT_RELEASE_REPO overrides it for testing.
var GitHubRepo = envOrDefault("RELKIT_RELEASE_REPO", "shichao402/relkit")

// UpgradeFallbackArtifacts lists the prebuilt binaries that remain pinned in
// consume/3 locks even though the updater itself exits the block (module
// channel covers it): every product-binary row except the updater.
var UpgradeFallbackArtifacts = envList("RELKIT_UPGRADE_FALLBACK_ARTIFACTS", "cli")

// ReleaseManifest is the immutable build metadata every GitHub release
// attachment pins: commit, host-scripts tree hash, protocol windows.
type ReleaseManifest struct {
	Commit         string `json:"commit"`
	HostScriptsSHA string `json:"hostScriptsSha256"`
	ConsumerSHA    string `json:"consumerSha256"`
	MinProtocol    int    `json:"minProtocol"`
	MaxProtocol    int    `json:"maxProtocol"`
	MinUpdaterIpc  int    `json:"minUpdaterIpc"`
	MaxUpdaterIpc  int    `json:"maxUpdaterIpc"`
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envList(key, fallback string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return strings.Split(fallback, ",")
	}
	return strings.Split(raw, ",")
}

// HTTPGet fetches a release attachment with the relkit-host user agent.
func HTTPGet(url string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "relkit-host")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("GET %s failed: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("GET %s failed: status %d", url, response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("GET %s failed: %w", url, err)
	}
	return string(body), nil
}

var (
	sha256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	commitPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	releasePattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
)

// ParseSHA256SUMS parses the SHA256SUMS attachment: "<hash>  <file>" lines.
func ParseSHA256SUMS(text string) map[string]string {
	mapping := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			mapping[strings.TrimPrefix(parts[len(parts)-1], "*")] = strings.ToLower(parts[0])
		}
	}
	return mapping
}

// ReleaseManifestFor fetches and parses manifest.json from the release CDN.
func ReleaseManifestFor(base string) (*ReleaseManifest, error) {
	raw, err := HTTPGet(base + "/manifest.json")
	if err != nil {
		return nil, err
	}
	m := &ReleaseManifest{}
	if err := json.Unmarshal([]byte(raw), m); err != nil {
		return nil, fmt.Errorf("%s/manifest.json is not JSON: %w", base, err)
	}
	return m, nil
}

// UpgradeLock rebuilds the whole consume/3 lock out of one immutable release.
// Constructing instead of patching is what lets a consume/1 or consume/2 repo
// run upgrade directly: every pinned value is restated by the release, so the
// old lock's shape is never a precondition.
func UpgradeLock(release string, manifest *ReleaseManifest, sums map[string]string, previous *Lock) (*Lock, error) {
	if !releasePattern.MatchString(release) {
		return nil, fmt.Errorf("upgrade expects vX.Y.Z")
	}
	commit := strings.ToLower(strings.TrimSpace(manifest.Commit))
	if !commitPattern.MatchString(commit) {
		return nil, fmt.Errorf("manifest.json must pin a 40-char commit; refusing to keep the previous lock commit")
	}
	hostTree := strings.ToLower(strings.TrimSpace(manifest.HostScriptsSHA))
	if !sha256Pattern.MatchString(hostTree) {
		return nil, fmt.Errorf("manifest.json has no valid hostScriptsSha256")
	}
	_ = strings.ToLower(strings.TrimSpace(manifest.ConsumerSHA))
	if _, ok := sums["relkit-host-scripts.zip"]; !ok {
		return nil, fmt.Errorf("SHA256SUMS has no relkit-host-scripts.zip")
	}

	base := ReleaseBase(release)
	artifacts := map[string]json.RawMessage{}
	for _, row := range registry.ProductComponents() {
		if row.Role != registry.RoleProductTree {
			continue
		}
		if sum, ok := sums[row.Archive]; ok {
			spec := ArtifactSpec{
				URLs:   []string{base + "/" + row.Archive, cnbAttachmentURL(release, row.Archive)},
				SHA256: sum,
			}
			encoded, err := json.Marshal(&spec)
			if err != nil {
				return nil, err
			}
			artifacts[row.Name] = encoded
		}
	}
	for _, name := range UpgradeFallbackArtifacts {
		row, ok := registry.ByName[name]
		if !ok || row.Role != registry.RoleProductBinary {
			continue
		}
		// Product binaries keep the by-target shape in consume/3 locks:
		// {target: {urls, sha256}}. Only the fallback rows (cli) are pinned;
		// the updater exits the block for the module channel.
		byTarget := map[string]ArtifactSpec{}
		for _, target := range registry.Targets {
			filename := row.ArtifactFilename(target)
			if sum, ok := sums[filename]; ok {
				byTarget[target] = ArtifactSpec{
					URLs:   []string{base + "/" + filename, cnbAttachmentURL(release, filename)},
					SHA256: sum,
				}
			}
		}
		if len(byTarget) > 0 {
			encoded, err := json.Marshal(&byTarget)
			if err != nil {
				return nil, err
			}
			artifacts[name] = json.RawMessage(encoded)
		}
	}

	lock := &Lock{
		Schema:  SchemaV3,
		Release: release,
		Commit:  commit,
		Source: &SourceBlock{
			Module:  "github.com/shichao402/relkit",
			Version: release,
			H1:      "",
			Commit:  commit,
		},
		HostScriptsSHA256: hostTree,
		Protocol:          intWindow(manifest.MinProtocol, manifest.MaxProtocol, PublishProtocolFallback),
		UpdaterIPC:        intWindow(manifest.MinUpdaterIpc, manifest.MaxUpdaterIpc, UpdaterIpcFallback),
		Artifacts:         artifacts,
	}
	return lock, nil
}

func intWindow(min, max, fallback int) IntWindow {
	if min <= 0 {
		min = fallback
	}
	if max <= min {
		max = min
	}
	return IntWindow{Min: min, Max: max}
}

// ReleaseBase is the GitHub release download base URL for release.
func ReleaseBase(release string) string {
	return "https://github.com/" + GitHubRepo + "/releases/download/" + release
}

// cnbAttachmentURL is the CNB mirror attachment URL for release/filename.
func cnbAttachmentURL(release, filename string) string {
	return "https://cnb.cool/" + GitHubRepo + "/-/releases/download/" + release + "/" + filename
}
