package sdk

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
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
	IndexURLs       []string // ignored when EntryURLs is non-empty
	EntryURLs       []string // signed directory bootstrap (SPEC §12.1 step 0 / §16)
	FallbackURLs    []string // optional; also filled from directory services
	TrustedKeys     TrustedKeys
	ClientSelectors map[string]string
	Fetcher         Fetcher
	StateStore      StateStore
	Policy          Policy

	// Deprecated: prefer StateStore. Still honored when StateStore is nil.
	LastSeenFallbackSequence *int64

	state *UpdateState
}

// UpdateAvailable is returned when a newer reachable version exists.
type UpdateAvailable struct {
	Target             *rupv2.VersionNode
	Manifest           *rupv2.Manifest
	Artifact           *rupv2.Artifact
	Mandatory          bool
	RemainingHops      int
	Sequence           int64
	PriorReleaseNotes  []PriorReleaseNotes
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
	UpToDate         bool
	CurrentIsYanked  bool
	Available        *UpdateAvailable
	Fallback         *FallbackRequired
	Throttled        bool
	NextAllowedAt    time.Time
	Sequence         int64
	Attempts         []string
	Err              error
}

// PriorReleaseNotes is a historical notes link collected from the index.
type PriorReleaseNotes struct {
	Version string
	Code    int64
	Notes   string
	NotesURL string
}

func (u *Updater) policy() Policy {
	p := u.Policy
	def := DefaultPolicy()
	if p.AfterSuccess <= 0 {
		p.AfterSuccess = def.AfterSuccess
	}
	if p.AfterFailure <= 0 {
		p.AfterFailure = def.AfterFailure
	}
	if p.DocumentTimeout <= 0 {
		p.DocumentTimeout = def.DocumentTimeout
	}
	if p.DownloadIdleTimeout <= 0 {
		p.DownloadIdleTimeout = def.DownloadIdleTimeout
	}
	if p.DownloadRetries <= 0 {
		p.DownloadRetries = def.DownloadRetries
	}
	if p.DownloadWorkers <= 0 {
		p.DownloadWorkers = def.DownloadWorkers
	}
	if p.DownloadChunkSize <= 0 {
		p.DownloadChunkSize = def.DownloadChunkSize
	}
	return p
}

func (u *Updater) fetcher() Fetcher {
	if u.Fetcher != nil {
		return u.Fetcher
	}
	p := u.policy()
	return &HTTPFetcher{DocumentTimeout: p.DocumentTimeout, IdleTimeout: p.DownloadIdleTimeout}
}

func (u *Updater) loadState() *UpdateState {
	if u.state != nil {
		return u.state
	}
	if u.StateStore != nil {
		st, err := u.StateStore.Load()
		if err != nil || st == nil {
			st = &UpdateState{}
		}
		u.state = st
		return st
	}
	st := &UpdateState{LastSeenFallbackSequence: u.LastSeenFallbackSequence}
	u.state = st
	return st
}

func (u *Updater) saveState() {
	if u.state == nil {
		return
	}
	if u.StateStore != nil {
		_ = u.StateStore.Save(u.state)
	}
	if u.state.LastSeenFallbackSequence != nil {
		u.LastSeenFallbackSequence = u.state.LastSeenFallbackSequence
	}
}

// Skip records that the user chose to skip a version code.
func (u *Updater) Skip(code int) {
	st := u.loadState()
	st.Skip(code)
	u.saveState()
}

// IsSkipped reports whether code is in the persisted skip list.
func (u *Updater) IsSkipped(code int) bool {
	return u.loadState().IsSkipped(code)
}

// Check fetches index envelopes, verifies signatures, and selects the next target.
// When force is false, SPEC §12.2 throttling may return Throttled.
func (u *Updater) Check(ctx context.Context) CheckResult {
	return u.CheckForce(ctx, false)
}

// CheckForce is Check with an explicit force flag (ignore throttle when true).
func (u *Updater) CheckForce(ctx context.Context, force bool) CheckResult {
	st := u.loadState()
	p := u.policy()
	if !force && !p.ShouldCheck(st.LastCheckAt, st.LastResult) {
		return CheckResult{
			Throttled:     true,
			NextAllowedAt: p.NextAllowedAt(st.LastCheckAt, st.LastResult),
		}
	}

	normal := u.checkIndex(ctx)
	fb, fbAttempts := u.CheckFallback(ctx)
	normal.Attempts = append(normal.Attempts, fbAttempts...)

	now := time.Now().UTC()
	st.LastCheckAt = &now
	switch {
	case normal.Available != nil:
		st.LastResult = "available"
		normal.Fallback = fb
		u.saveState()
		return normal
	case fb != nil:
		st.LastResult = "fallback"
		u.saveState()
		return CheckResult{
			Fallback: fb,
			Attempts: normal.Attempts,
			Sequence: fb.Sequence,
		}
	case normal.UpToDate:
		st.LastResult = "success"
		u.saveState()
		return normal
	case normal.Err != nil:
		st.LastResult = "failure"
		u.saveState()
		return normal
	default:
		st.LastResult = "success"
		u.saveState()
		return normal
	}
}

type indexCandidate struct {
	URL            string
	PreferenceKey  string
	FallbackURL    string
}

func (u *Updater) resolveIndexPlan(ctx context.Context) (candidates []indexCandidate, fallbackURLs []string, attempts []string) {
	fallbackURLs = append([]string{}, u.FallbackURLs...)
	st := u.loadState()

	if len(u.EntryURLs) > 0 {
		entries := RankURLStrings(u.EntryURLs, st)
		for _, entryURL := range entries {
			doc, err := u.loadDirectory(ctx, entryURL)
			if err != nil {
				attempts = append(attempts, fmt.Sprintf("%s: %v", entryURL, err))
				st.RecordSourceFailure(entryURL)
				continue
			}
			st.RecordSourceSuccess(entryURL, 0)
			st.ObserveDirectorySequence(doc.DirectorySequence)
			services := servicesForChannel(doc, u.Channel)
			for _, svc := range services {
				if svc.IndexUrl == "" {
					continue
				}
				candidates = append(candidates, indexCandidate{
					URL:           svc.IndexUrl,
					PreferenceKey: DirectoryServiceKey(svc.Id),
					FallbackURL:   svc.FallbackUrl,
				})
				if svc.FallbackUrl != "" {
					fallbackURLs = append(fallbackURLs, svc.FallbackUrl)
				}
			}
			if len(candidates) > 0 {
				break
			}
			attempts = append(attempts, fmt.Sprintf("%s: no services for channel %q", entryURL, u.Channel))
		}
		candidates = RankByLearning(candidates, func(c indexCandidate) string { return c.PreferenceKey }, st)
		return candidates, uniqueStrings(fallbackURLs), attempts
	}

	for _, url := range RankURLStrings(u.IndexURLs, st) {
		candidates = append(candidates, indexCandidate{URL: url, PreferenceKey: url})
	}
	return candidates, uniqueStrings(fallbackURLs), attempts
}

func (u *Updater) loadDirectory(ctx context.Context, rawURL string) (*rupv2.UpdateDirectory, error) {
	body, err := u.fetcher().GetBytes(ctx, bustCache(rawURL))
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	env, err := rupv2.UnmarshalEnvelope(body)
	if err != nil {
		return nil, fmt.Errorf("envelope: %w", err)
	}
	doc, err := envelope.OpenDirectoryEnvelope(env, u.TrustedKeys)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}
	if doc.Product != u.Product {
		return nil, fmt.Errorf("product mismatch")
	}
	st := u.loadState()
	if !AcceptsSequence(doc.DirectorySequence, st.LastSeenDirectorySequence) {
		return nil, fmt.Errorf("directory sequence %d older than last seen", doc.DirectorySequence)
	}
	return doc, nil
}

func servicesForChannel(doc *rupv2.UpdateDirectory, channel string) []*rupv2.DirectoryService {
	var matched []*rupv2.DirectoryService
	for _, svc := range doc.Services {
		if svc == nil {
			continue
		}
		if svc.Channel != "" && svc.Channel != channel {
			continue
		}
		matched = append(matched, svc)
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].Priority != matched[j].Priority {
			return matched[i].Priority < matched[j].Priority
		}
		return matched[i].Id < matched[j].Id
	})
	return matched
}

func (u *Updater) checkIndex(ctx context.Context) CheckResult {
	if u.Product == "" || u.Channel == "" {
		return CheckResult{Err: fmt.Errorf("product and channel are required")}
	}
	if len(u.TrustedKeys) == 0 {
		return CheckResult{Err: fmt.Errorf("trusted keys are required")}
	}
	if len(u.EntryURLs) == 0 && len(u.IndexURLs) == 0 {
		return CheckResult{Err: fmt.Errorf("at least one entry URL or index URL is required")}
	}

	st := u.loadState()
	candidates, _, attempts := u.resolveIndexPlan(ctx)
	if len(candidates) == 0 {
		return CheckResult{
			Attempts: attempts,
			Err:      fmt.Errorf("no usable directory/index source (%d attempted)", len(attempts)),
		}
	}

	for _, cand := range candidates {
		indexURL := bustCache(cand.URL)
		body, err := u.fetcher().GetBytes(ctx, indexURL)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: fetch: %v", cand.URL, err))
			st.RecordSourceFailure(cand.PreferenceKey)
			continue
		}
		env, err := rupv2.UnmarshalEnvelope(body)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: envelope: %v", cand.URL, err))
			st.RecordSourceFailure(cand.PreferenceKey)
			continue
		}
		index, err := envelope.OpenEnvelope(env, u.TrustedKeys)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: verify: %v", cand.URL, err))
			st.RecordSourceFailure(cand.PreferenceKey)
			continue
		}
		if index.Product != u.Product || index.Channel != u.Channel {
			attempts = append(attempts, fmt.Sprintf("%s: product/channel mismatch", cand.URL))
			st.RecordSourceFailure(cand.PreferenceKey)
			continue
		}
		if !AcceptsSequence(index.Sequence, st.LastSeenSequence) {
			attempts = append(attempts, fmt.Sprintf("%s: sequence %d older than last seen", cand.URL, index.Sequence))
			continue
		}

		st.RecordSourceSuccess(cand.PreferenceKey, 0)
		st.ObserveSequence(index.Sequence)

		yanked := currentIsYanked(index, u.CurrentCode)
		path := chain.ResolveUpgradePath(index, u.CurrentCode)
		if len(path) == 0 {
			return CheckResult{UpToDate: true, CurrentIsYanked: yanked, Sequence: index.Sequence, Attempts: attempts}
		}
		target := path[0]
		if !chain.IsMandatory(index, u.CurrentCode) && st.IsSkipped(int(target.Code)) {
			return CheckResult{UpToDate: true, CurrentIsYanked: yanked, Sequence: index.Sequence, Attempts: attempts}
		}
		manifest, artifact, err := u.fetchTarget(ctx, index, &target)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: target: %v", cand.URL, err))
			continue
		}
		return CheckResult{
			Available: &UpdateAvailable{
				Target:            &target,
				Manifest:          manifest,
				Artifact:          artifact,
				Mandatory:         chain.IsMandatory(index, u.CurrentCode),
				RemainingHops:     len(path),
				Sequence:          index.Sequence,
				PriorReleaseNotes: collectPriorReleaseNotes(index, u.CurrentCode),
			},
			CurrentIsYanked: yanked,
			Sequence:        index.Sequence,
			Attempts:        attempts,
		}
	}
	return CheckResult{
		Attempts: attempts,
		Err:      fmt.Errorf("no usable index source (%d attempted)", len(attempts)),
	}
}

func currentIsYanked(index *rupv2.Index, currentCode int) bool {
	for _, v := range index.Versions {
		if v != nil && int(v.Code) == currentCode {
			return v.Yanked
		}
	}
	return false
}

func collectPriorReleaseNotes(index *rupv2.Index, currentCode int) []PriorReleaseNotes {
	var out []PriorReleaseNotes
	for _, v := range index.Versions {
		if v == nil || int(v.Code) <= currentCode {
			continue
		}
		if v.Notes == "" && v.NotesUrl == "" {
			continue
		}
		out = append(out, PriorReleaseNotes{
			Version:  v.Version,
			Code:     v.Code,
			Notes:    v.Notes,
			NotesURL: v.NotesUrl,
		})
	}
	return out
}

// CheckFallback evaluates only the signed fallback document (SPEC §12.6).
func (u *Updater) CheckFallback(ctx context.Context) (*FallbackRequired, []string) {
	_, fallbackURLs, planAttempts := u.resolveIndexPlan(ctx)
	urls := fallbackURLs
	if len(urls) == 0 {
		urls = u.FallbackURLs
	}
	if len(urls) == 0 {
		return nil, planAttempts
	}
	if u.Product == "" {
		return nil, append(planAttempts, "product is required for fallback")
	}
	if len(u.TrustedKeys) == 0 {
		return nil, append(planAttempts, "trusted keys are required for fallback")
	}

	st := u.loadState()
	var attempts []string
	attempts = append(attempts, planAttempts...)
	for _, rawURL := range RankURLStrings(urls, st) {
		body, err := u.fetcher().GetBytes(ctx, bustCache(rawURL))
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
		if !AcceptsSequence(doc.Sequence, st.LastSeenFallbackSequence) {
			attempts = append(attempts, fmt.Sprintf("%s: sequence %d older than last seen", rawURL, doc.Sequence))
			continue
		}
		st.ObserveFallbackSequence(doc.Sequence)

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

func matchFallbackRule(doc *rupv2.Fallback, currentCode int64, clientSelectors map[string]string) *rupv2.FallbackRule {
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

func (u *Updater) fetchTarget(ctx context.Context, index *rupv2.Index, target *rupv2.VersionNode) (*rupv2.Manifest, *rupv2.Artifact, error) {
	if target.Manifest == nil || len(target.Manifest.Urls) == 0 {
		return nil, nil, fmt.Errorf("target has no manifest urls")
	}
	st := u.loadState()
	urls := RankURLStrings(target.Manifest.Urls, st)
	var last error
	for _, url := range urls {
		body, err := u.fetcher().GetBytes(ctx, url)
		if err != nil {
			last = err
			st.RecordSourceFailure(url)
			continue
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != target.Manifest.Sha256 {
			last = fmt.Errorf("manifest sha256 mismatch")
			st.RecordSourceFailure(url)
			continue
		}
		if int64(len(body)) != target.Manifest.Size {
			last = fmt.Errorf("manifest size mismatch")
			st.RecordSourceFailure(url)
			continue
		}
		manifest, err := rupv2.UnmarshalManifest(body)
		if err != nil {
			last = err
			st.RecordSourceFailure(url)
			continue
		}
		if manifest.Product != index.Product || manifest.Version != target.Version || manifest.Code != target.Code {
			last = fmt.Errorf("manifest identity mismatch")
			st.RecordSourceFailure(url)
			continue
		}
		artifact := selectors.SelectArtifact(manifest, u.ClientSelectors)
		if artifact == nil {
			last = fmt.Errorf("no artifact matches selectors")
			st.RecordSourceFailure(url)
			continue
		}
		st.RecordSourceSuccess(url, 0)
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
	st := u.loadState()
	urls := RankURLStrings(available.Artifact.Urls, st)
	art := *available.Artifact
	art.Urls = urls
	verified, err := DownloadArtifact(ctx, u.fetcher(), &art, destPath, u.policy(), nil)
	if err != nil {
		for _, url := range urls {
			st.RecordSourceFailure(url)
		}
		u.saveState()
		return err
	}
	if verified.SourceURL != "" {
		st.RecordSourceSuccess(verified.SourceURL, 0)
	}
	u.saveState()
	return nil
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
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
