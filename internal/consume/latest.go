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
)

// LatestKeyword is the lock release value that means "follow the newest
// published release" instead of pinning one immutable version.
const LatestKeyword = "latest"

// IsLatest reports whether a release value selects the follow-latest mode.
func IsLatest(release string) bool {
	return strings.TrimSpace(strings.ToLower(release)) == LatestKeyword
}

// LatestReleaseRes is the subset of the GitHub latest-release API document
// ResolveLatest consumes: tag_name plus html_url for diagnostics.
type LatestReleaseRes struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// ResolveLatest asks GitHub for the newest published release tag of
// GitHubRepo (api.github.com/releases/latest, anonymous). The consumer keeps
// RELKIT_RELEASE_REPO as the override hook for tests.
func ResolveLatest() (string, error) {
	url := "https://api.github.com/repos/" + GitHubRepo + "/releases/latest"
	client := &http.Client{Timeout: 30 * time.Second}
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "relkit-host")
	request.Header.Set("Accept", "application/vnd.github+json")
	if token := strings.TrimSpace(latestToken()); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
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
	doc := &LatestReleaseRes{}
	if err := json.Unmarshal(body, doc); err != nil {
		return "", fmt.Errorf("%s is not JSON: %w", url, err)
	}
	tag := strings.TrimSpace(doc.TagName)
	if !releasePattern.MatchString(tag) {
		return "", fmt.Errorf("latest release tag %q is not vX.Y.Z", tag)
	}
	return tag, nil
}

// latestToken surfaces GITHUB_TOKEN for rate-limited environments. The
// anonymous quota (60 req/h per IP) is one call per install; products that
// resolve often (every CI run) can set the token when the pool IP is shared.
func latestToken() string {
	return strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
}

// MirrorBase is relkit's own mirrors partition mirroring the GitHub release
// assets (ADR 0018: the consumer-driven sync pipeline PUTs each release
// there). RELKIT_MIRROR_BASE overrides it for tests and self-hosted forks.
var MirrorBase = envOrDefault("RELKIT_MIRROR_BASE",
	"https://mirrors.tencent.com/repository/generic/relkit/mirror/github/relkit")

// MirrorAssetURL is the mirrors URL for one release asset file.
func MirrorAssetURL(release, filename string) string {
	return MirrorBase + "/" + release + "/" + filename
}

// ResolveLatestViaRedirect follows github.com/<repo>/releases/latest, which
// 302-redirects to releases/tag/<tag>. The release-download face carries no
// api.github.com quota (60 req/h anonymous), which makes it the primary
// resolution path; the API stays as a second opinion and the mirrors
// listing as the GitHub-unreachable fallback.
func ResolveLatestViaRedirect() (string, error) {
	url := "https://github.com/" + GitHubRepo + "/releases/latest"
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
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
	if response.StatusCode < 300 || response.StatusCode >= 400 {
		return "", fmt.Errorf("GET %s returned status %d", url, response.StatusCode)
	}
	tag, ok := tagFromRedirectLocation(response.Header.Get("Location"))
	if !ok {
		return "", fmt.Errorf("latest redirect %q does not point at a vX.Y.Z tag", response.Header.Get("Location"))
	}
	return tag, nil
}

// tagFromRedirectLocation extracts the release tag from a
// releases/tag/<tag> redirect target.
func tagFromRedirectLocation(location string) (string, bool) {
	trimmed := strings.TrimSpace(location)
	i := strings.Index(trimmed, "/tag/")
	if i < 0 {
		return "", false
	}
	tag := strings.Trim(trimmed[i+len("/tag/"):], "/")
	if !releasePattern.MatchString(tag) {
		return "", false
	}
	return tag, true
}

// mirrorDirPattern matches the directory-listing anchors the mirrors
// generic repository serves ("Index of" HTML): <a href="v0.5.18/">v0.5.18/</a>.
var mirrorDirPattern = regexp.MustCompile(`href="(v\d+\.\d+\.\d+)/?"`)

// ResolveLatestViaMirrors lists the mirrors relkit partition and returns the
// newest vX.Y.Z generation. This is the fallback for networks where GitHub
// itself is unreachable (intranet build pools): the consumer-driven sync
// pipeline keeps the partition current, so the newest synced generation is
// the latest release as far as that network can see.
func ResolveLatestViaMirrors() (string, error) {
	body, err := HTTPGet(MirrorBase + "/")
	if err != nil {
		return "", err
	}
	best := ""
	for _, match := range mirrorDirPattern.FindAllStringSubmatch(body, -1) {
		tag := match[1]
		if best == "" || semverLess(best, tag) {
			best = tag
		}
	}
	if best == "" {
		return "", fmt.Errorf("mirrors listing %s has no vX.Y.Z directories", MirrorBase)
	}
	return best, nil
}

// ResolveLatestTag is the full follow-latest resolution chain: GitHub
// release redirect (no API quota), then the releases API (token-aware),
// then the mirrors listing (GitHub-unreachable networks). Each step fails
// fast so hostile networks reach the mirrors fallback quickly.
func ResolveLatestTag() (string, error) {
	if tag, err := ResolveLatestViaRedirect(); err == nil {
		return tag, nil
	}
	if tag, err := ResolveLatest(); err == nil {
		return tag, nil
	}
	return ResolveLatestViaMirrors()
}

func semverLess(a, b string) bool {
	pa, pb := semverTuple(a), semverTuple(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func semverTuple(v string) [3]int {
	var out [3]int
	for i, part := range strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3) {
		n := 0
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				break
			}
			n = n*10 + int(ch-'0')
		}
		out[i] = n
	}
	return out
}
