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
	FallbackURLs    []string // optional; empty disables fallback check
	TrustedKeys     TrustedKeys
	ClientSelectors map[string]string
	HTTPClient      *http.Client

	// LastSeenFallbackSequence is the highest fallback sequence this client has
	// accepted. Persist it across runs (product-scoped) to reject replays.
	LastSeenFallbackSequence *int64
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

// FallbackRequired urges a manual update via a signed rule (SPEC §12.6).
type FallbackRequired struct {
	ManualURL string
	Message   string
	Mandatory bool
	Sequence  int64
	MinCode   int64
	MaxCode   int64
}

// CheckResult is the outcome of Check.
type CheckResult struct {
	UpToDate  bool
	Available *UpdateAvailable
	Fallback  *FallbackRequired
	Sequence  int64
	Attempts  []string
	Err       error
}

// Check fetches index envelopes, verifies signatures, and selects the next target.
// When FallbackURLs is configured, also evaluates the fallback document and
// merges results: Available > Fallback > UpToDate / error.
func (u *Updater) Check(ctx context.Context) CheckResult {
	normal := u.checkIndex(ctx)
	fb, fbAttempts := u.CheckFallback(ctx)

	if normal.Available != nil {
		normal.Fallback = fb
		return normal
	}
	if fb != nil {
		return CheckResult{
			Fallback: fb,
			Attempts: append(append([]string{}, normal.Attempts...), fbAttempts...),
		}
	}
	return normal
}

func (u *Updater) checkIndex(ctx context.Context) CheckResult {
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

// CheckFallback evaluates only the signed fallback document (SPEC §12.6).
// Returns nil when no URLs are configured, no rule matches, or no source works.
func (u *Updater) CheckFallback(ctx context.Context) (*FallbackRequired, []string) {
	if len(u.FallbackURLs) == 0 {
		return nil, nil
	}
	if u.Product == "" {
		return nil, []string{"product is required for fallback"}
	}
	if len(u.TrustedKeys) == 0 {
		return nil, []string{"trusted keys are required for fallback"}
	}

	client := u.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	var attempts []string
	for _, rawURL := range u.FallbackURLs {
		fbURL := bustCache(rawURL)
		body, err := getBytes(ctx, client, fbURL)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: fetch: %v", rawURL, err))
			continue
		}
		env, err := rupv2.UnmarshalEnvelope(body)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: envelope: %v", rawURL, err))
			continue
		}
		doc, err := envelope.OpenFallbackEnvelope(env, u.TrustedKeys)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: verify: %v", rawURL, err))
			continue
		}
		if doc.Product != u.Product {
			attempts = append(attempts, fmt.Sprintf("%s: product mismatch", rawURL))
			continue
		}
		if u.LastSeenFallbackSequence != nil && doc.Sequence < *u.LastSeenFallbackSequence {
			attempts = append(attempts, fmt.Sprintf("%s: sequence %d older than last seen %d", rawURL, doc.Sequence, *u.LastSeenFallbackSequence))
			continue
		}

		if u.LastSeenFallbackSequence == nil || doc.Sequence > *u.LastSeenFallbackSequence {
			seq := doc.Sequence
			u.LastSeenFallbackSequence = &seq
		}

		rule := matchFallbackRule(doc, int64(u.CurrentCode), u.ClientSelectors)
		if rule == nil {
			return nil, attempts
		}
		return &FallbackRequired{
			ManualURL: rule.ManualUrl,
			Message:   rule.Message,
			Mandatory: rule.Mandatory,
			Sequence:  doc.Sequence,
			MinCode:   rule.MinCode,
			MaxCode:   rule.MaxCode,
		}, attempts
	}
	return nil, attempts
}

func matchFallbackRule(doc *model.FallbackDocument, currentCode int64, clientSelectors map[string]string) *model.FallbackRule {
	for _, rule := range doc.Rules {
		if rule == nil {
			continue
		}
		if currentCode < rule.MinCode || currentCode > rule.MaxCode {
			continue
		}
		if !selectors.MatchesClient(model.SelectorsToMap(rule.Selectors), clientSelectors) {
			continue
		}
		return rule
	}
	return nil
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
