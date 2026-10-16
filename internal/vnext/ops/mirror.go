package ops

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/jsonio"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/catalog"
)

const maxMirrorAssetSize int64 = 16 << 30

type MirrorDistributionOptions struct {
	DistributionPath  string
	MirrorRoot        string
	ReleaseAssetsBase string
	Username          string
	Password          string
	HTTPClient        *http.Client
}

type MirroredAsset struct {
	Digest    string `json:"digest"`
	SourceURL string `json:"sourceUrl"`
	MirrorURL string `json:"mirrorUrl"`
	Size      int64  `json:"size"`
	Status    string `json:"status"`
}

type MirrorDistributionResult struct {
	Schema             string          `json:"schema"`
	Version            string          `json:"version"`
	DistributionDigest string          `json:"distributionDigest"`
	MirrorRoot         string          `json:"mirrorRoot"`
	Assets             []MirroredAsset `json:"assets"`
}

// MirrorDistribution copies every release asset in one immutable distribution
// into a content-addressed mirror. Selection remains controlled by the signed
// Catalog and Operator Profile; the mirror is only a byte supply.
func MirrorDistribution(ctx context.Context, options MirrorDistributionOptions) (*MirrorDistributionResult, error) {
	var distribution catalog.Distribution
	if err := jsonio.LoadPath(options.DistributionPath, &distribution); err != nil {
		return nil, err
	}
	if distribution.Version == "" || distribution.Commit == "" || len(distribution.Components) == 0 {
		return nil, fmt.Errorf("distribution is missing version, commit, or components")
	}
	if err := canonical.ValidateDigest(distribution.Digest); err != nil {
		return nil, fmt.Errorf("distribution digest: %w", err)
	}
	identity, err := distribution.IdentityDigest()
	if err != nil {
		return nil, err
	}
	if identity != distribution.Digest {
		return nil, fmt.Errorf("distribution identity is %s, expected %s", identity, distribution.Digest)
	}
	root, err := validateMirrorRoot(options.MirrorRoot)
	if err != nil {
		return nil, err
	}
	if options.Username == "" || options.Password == "" {
		return nil, fmt.Errorf("mirror upload credentials are required")
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Minute}
	}

	sources := map[string]string{}
	for index, component := range distribution.Components {
		if err := canonical.ValidateDigest(component.SHA256); err != nil {
			return nil, fmt.Errorf("component %d digest: %w", index, err)
		}
		if component.Source.Kind != "release-asset" {
			continue
		}
		if component.Source.URL == "" {
			return nil, fmt.Errorf("component %s/%s has no release asset URL", component.Name, component.Platform)
		}
		if _, exists := sources[component.SHA256]; !exists {
			sources[component.SHA256] = component.Source.URL
		}
	}
	if len(distribution.ReleaseAssets) > 0 {
		base, err := validateReleaseAssetsBase(options.ReleaseAssetsBase)
		if err != nil {
			return nil, err
		}
		for name, digest := range distribution.ReleaseAssets {
			if name == "" || name != path.Base(name) || strings.Contains(name, `\`) {
				return nil, fmt.Errorf("release asset name %q is not a safe file name", name)
			}
			if err := canonical.ValidateDigest(digest); err != nil {
				return nil, fmt.Errorf("release asset %s digest: %w", name, err)
			}
			if _, exists := sources[digest]; !exists {
				sources[digest] = base + "/" + url.PathEscape(name)
			}
		}
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("distribution has no release-asset components")
	}

	digests := make([]string, 0, len(sources))
	for digest := range sources {
		digests = append(digests, digest)
	}
	sort.Strings(digests)
	result := &MirrorDistributionResult{
		Schema:             "relkit.ops-mirror-result/1",
		Version:            distribution.Version,
		DistributionDigest: distribution.Digest,
		MirrorRoot:         root,
		Assets:             make([]MirroredAsset, 0, len(digests)),
	}
	temporary, err := os.MkdirTemp("", "relkit-mirror-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	for _, digest := range digests {
		asset, err := mirrorAsset(ctx, client, temporary, root, digest, sources[digest], options.Username, options.Password)
		if err != nil {
			return nil, err
		}
		result.Assets = append(result.Assets, *asset)
	}
	return result, nil
}

func validateReleaseAssetsBase(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("release assets base must be an HTTPS URL without credentials, query, or fragment")
	}
	return value, nil
}

func validateMirrorRoot(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("mirror root must be an HTTP(S) URL without credentials, query, or fragment")
	}
	return value, nil
}

func mirrorAsset(
	ctx context.Context,
	client *http.Client,
	temporary string,
	root string,
	digest string,
	sourceURL string,
	username string,
	password string,
) (*MirroredAsset, error) {
	hash := strings.TrimPrefix(digest, canonical.DigestPrefix)
	sourcePath := filepath.Join(temporary, hash)
	size, err := downloadVerifiedFile(ctx, client, sourceURL, sourcePath, digest)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", sourceURL, err)
	}
	mirrorURL := root + "/blobs/" + hash
	status, err := uploadMirrorFile(ctx, client, mirrorURL, sourcePath, size, username, password)
	if err != nil {
		return nil, fmt.Errorf("upload %s: %w", mirrorURL, err)
	}
	if _, err := verifyRemoteDigest(ctx, client, mirrorURL, digest, username, password); err != nil {
		return nil, fmt.Errorf("verify %s: %w", mirrorURL, err)
	}
	return &MirroredAsset{
		Digest: digest, SourceURL: sourceURL, MirrorURL: mirrorURL, Size: size, Status: status,
	}, nil
}

func downloadVerifiedFile(ctx context.Context, client *http.Client, rawURL, destination, expected string) (int64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return 0, fmt.Errorf("source returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxMirrorAssetSize {
		return 0, fmt.Errorf("source asset exceeds %d bytes", maxMirrorAssetSize)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	hasher := sha256.New()
	size, copyErr := io.Copy(
		io.MultiWriter(output, hasher),
		io.LimitReader(response.Body, maxMirrorAssetSize+1),
	)
	closeErr := output.Close()
	if copyErr != nil {
		return 0, copyErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if size > maxMirrorAssetSize {
		return 0, fmt.Errorf("source asset exceeds %d bytes", maxMirrorAssetSize)
	}
	actual := canonical.DigestPrefix + fmt.Sprintf("%x", hasher.Sum(nil))
	if actual != expected {
		return 0, fmt.Errorf("digest is %s, expected %s", actual, expected)
	}
	return size, nil
}

func uploadMirrorFile(
	ctx context.Context,
	client *http.Client,
	rawURL string,
	sourcePath string,
	size int64,
	username string,
	password string,
) (string, error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer source.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, source)
	if err != nil {
		return "", err
	}
	request.ContentLength = size
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("If-None-Match", "*")
	request.Header.Set("X-BKREPO-EXPIRES", "0")
	request.SetBasicAuth(username, password)
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	switch {
	case response.StatusCode == http.StatusConflict || response.StatusCode == http.StatusPreconditionFailed:
		return "present", nil
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return "uploaded", nil
	default:
		return "", fmt.Errorf("mirror returned HTTP %d", response.StatusCode)
	}
}

func verifyRemoteDigest(
	ctx context.Context,
	client *http.Client,
	rawURL string,
	expected string,
	username string,
	password string,
) (int64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	if username != "" || password != "" {
		request.SetBasicAuth(username, password)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return 0, fmt.Errorf("mirror readback returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxMirrorAssetSize {
		return 0, fmt.Errorf("mirror asset exceeds %d bytes", maxMirrorAssetSize)
	}
	hasher := sha256.New()
	size, err := io.Copy(hasher, io.LimitReader(response.Body, maxMirrorAssetSize+1))
	if err != nil {
		return 0, err
	}
	if size > maxMirrorAssetSize {
		return 0, fmt.Errorf("mirror asset exceeds %d bytes", maxMirrorAssetSize)
	}
	actual := canonical.DigestPrefix + fmt.Sprintf("%x", hasher.Sum(nil))
	if actual != expected {
		return 0, fmt.Errorf("readback digest is %s, expected %s", actual, expected)
	}
	return size, nil
}
