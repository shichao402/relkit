package consume

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
