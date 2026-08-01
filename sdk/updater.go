package sdk

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/chain"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/selectors"
)

// TrustedKeys maps keyId -> raw 32-byte ed25519 public key.
type TrustedKeys map[string]ed25519.PublicKey

// Updater checks for and downloads RUP v2 updates.
type Updater struct {
	Product         string
	Channel         string
	CurrentCode     int
	IndexURLs       []string
	TrustedKeys     TrustedKeys
	ClientSelectors map[string]string
	HTTPClient      *http.Client
}

// UpdateAvailable is returned when a newer reachable version exists.
type UpdateAvailable struct {
	Target        *model.VersionNode
	Manifest      *model.ManifestDocument
	Artifact      *model.ManifestArtifact
	Mandatory     bool
	RemainingHops int
	Sequence      int64
}

// CheckResult is the outcome of Check.
type CheckResult struct {
	UpToDate  bool
	Available *UpdateAvailable
	Sequence  int64
	Attempts  []string
	Err       error
}

// Check fetches index envelopes, verifies signatures, and selects the next target.
func (u *Updater) Check(ctx context.Context) CheckResult {
	if u.Product == "" || u.Channel == "" {
		return CheckResult{Err: fmt.Errorf("product and channel are required")}
	}
	if len(u.IndexURLs) == 0 {
		return CheckResult{Err: fmt.Errorf("at least one index URL is required")}
	}
	if len(u.TrustedKeys) == 0 {
		return CheckResult{Err: fmt.Errorf("trusted keys are required")}
	}

	client := u.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	var attempts []string
	for _, rawURL := range u.IndexURLs {
		indexURL := bustCache(rawURL)
		body, err := getBytes(ctx, client, indexURL)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: fetch: %v", rawURL, err))
			continue
		}
		env, err := rupv2.UnmarshalEnvelope(body)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: envelope: %v", rawURL, err))
			continue
		}
		index, err := envelope.OpenEnvelope(env, u.TrustedKeys)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: verify: %v", rawURL, err))
			continue
		}
		if index.Product != u.Product || index.Channel != u.Channel {
			attempts = append(attempts, fmt.Sprintf("%s: product/channel mismatch", rawURL))
			continue
		}

		path := chain.ResolveUpgradePath(index, u.CurrentCode)
		if len(path) == 0 {
			return CheckResult{UpToDate: true, Sequence: index.Sequence}
		}
		target := path[0]
		manifest, artifact, err := u.fetchTarget(ctx, client, index, &target)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: target: %v", rawURL, err))
			continue
		}
		return CheckResult{
			Available: &UpdateAvailable{
				Target:        &target,
				Manifest:      manifest,
				Artifact:      artifact,
				Mandatory:     chain.IsMandatory(index, u.CurrentCode),
				RemainingHops: len(path),
				Sequence:      index.Sequence,
			},
			Sequence: index.Sequence,
		}
	}
	return CheckResult{
		Attempts: attempts,
		Err:      fmt.Errorf("no usable index source (%d attempted)", len(attempts)),
	}
}

func (u *Updater) fetchTarget(ctx context.Context, client *http.Client, index *model.IndexDocument, target *model.VersionNode) (*model.ManifestDocument, *model.ManifestArtifact, error) {
	if target.Manifest == nil || len(target.Manifest.Urls) == 0 {
		return nil, nil, fmt.Errorf("target has no manifest urls")
	}
	var last error
	for _, url := range target.Manifest.Urls {
		body, err := getBytes(ctx, client, url)
		if err != nil {
			last = err
			continue
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != target.Manifest.Sha256 {
			last = fmt.Errorf("manifest sha256 mismatch")
			continue
		}
		if int64(len(body)) != target.Manifest.Size {
			last = fmt.Errorf("manifest size mismatch")
			continue
		}
		manifest, err := rupv2.UnmarshalManifest(body)
		if err != nil {
			last = err
			continue
		}
		if manifest.Product != index.Product || manifest.Version != target.Version || manifest.Code != target.Code {
			last = fmt.Errorf("manifest identity mismatch")
			continue
		}
		artifact := selectors.SelectArtifact(manifest, u.ClientSelectors)
		if artifact == nil {
			last = fmt.Errorf("no artifact matches selectors")
			continue
		}
		return manifest, artifact, nil
	}
	if last == nil {
		last = fmt.Errorf("all manifest mirrors failed")
	}
	return nil, nil, last
}

// Download writes the selected artifact to destPath after verifying sha256.
func (u *Updater) Download(ctx context.Context, available *UpdateAvailable, destPath string) error {
	if available == nil || available.Artifact == nil {
		return fmt.Errorf("nil update")
	}
	client := u.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	var last error
	for _, url := range available.Artifact.Urls {
		if err := downloadAndVerify(ctx, client, url, available.Artifact.Sha256, available.Artifact.Size, destPath); err != nil {
			last = err
			continue
		}
		return nil
	}
	if last == nil {
		last = fmt.Errorf("all artifact mirrors failed")
	}
	return last
}

func downloadAndVerify(ctx context.Context, client *http.Client, rawURL, wantSHA string, wantSize int64, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), resp.Body)
	if err != nil {
		return err
	}
	if n != wantSize {
		return fmt.Errorf("size mismatch: got %d want %d", n, wantSize)
	}
	if hex.EncodeToString(h.Sum(nil)) != wantSHA {
		return fmt.Errorf("sha256 mismatch")
	}
	return nil
}

func getBytes(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

func bustCache(rawURL string) string {
	sep := "?"
	for i := 0; i < len(rawURL); i++ {
		if rawURL[i] == '?' {
			sep = "&"
			break
		}
	}
	return fmt.Sprintf("%s%st=%d", rawURL, sep, time.Now().Unix())
}
