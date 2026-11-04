package agentpublish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/publishproto"
)

// ReleaseCodeState is the agent's answer on /v1/release-code (ADR 0025).
type ReleaseCodeState struct {
	Product          string `json:"product"`
	Code             int64  `json:"code,omitempty"`
	LastAllocated    int64  `json:"lastAllocated"`
	HighestPublished int64  `json:"highestPublished"`
}

// ReleaseCodeOptions addresses one product on one agent.
type ReleaseCodeOptions struct {
	Product  string
	AgentURL string
	Token    string
	Client   *http.Client
}

// ErrReleaseCodeUnsupported means the agent predates ADR 0025.
var ErrReleaseCodeUnsupported = errors.New("agent does not serve /v1/release-code (upgrade the server to a relkit with ADR 0025)")

// ClaimReleaseCode asks the agent for the next release code of a product. The
// returned code is fresh for this product on every channel; a claim that is
// never released only leaves a gap.
func ClaimReleaseCode(ctx context.Context, options ReleaseCodeOptions) (*ReleaseCodeState, error) {
	payload, err := json.Marshal(map[string]string{"product": options.Product})
	if err != nil {
		return nil, err
	}
	state, err := releaseCodeCall(ctx, options, http.MethodPost, normalizeEndpoint(options.AgentURL, "release-code"), payload)
	if err != nil {
		return nil, err
	}
	if state.Code <= 0 {
		return nil, fmt.Errorf("agent release-code claim returned no code")
	}
	return state, nil
}

// ReadReleaseCode reads the counter and the published floor without claiming.
func ReadReleaseCode(ctx context.Context, options ReleaseCodeOptions) (*ReleaseCodeState, error) {
	endpoint := normalizeEndpoint(options.AgentURL, "release-code") + "?product=" + url.QueryEscape(options.Product)
	return releaseCodeCall(ctx, options, http.MethodGet, endpoint, nil)
}

func releaseCodeCall(ctx context.Context, options ReleaseCodeOptions, method, endpoint string, payload []byte) (*ReleaseCodeState, error) {
	if strings.TrimSpace(options.Product) == "" {
		return nil, fmt.Errorf("product is required")
	}
	if strings.TrimSpace(options.AgentURL) == "" {
		return nil, fmt.Errorf("agent endpoint is required")
	}
	if strings.TrimSpace(options.Token) == "" {
		return nil, fmt.Errorf("release-code needs the product token")
	}
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+options.Token)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	publishproto.Apply(request.Header)
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: time.Minute}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("agent release-code %s: %w", endpoint, err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode == http.StatusNotFound && !looksLikeReleaseCodeError(raw) {
		return nil, ErrReleaseCodeUnsupported
	}
	if response.StatusCode != http.StatusOK {
		if protocolErr := publishproto.Explain(response.StatusCode, raw); protocolErr != nil {
			return nil, protocolErr
		}
		return nil, fmt.Errorf("agent release-code returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	var state ReleaseCodeState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("agent release-code response is not JSON: %w", err)
	}
	if state.Product != options.Product {
		return nil, fmt.Errorf("agent release-code answered for product %q, asked %q", state.Product, options.Product)
	}
	return &state, nil
}

// looksLikeReleaseCodeError tells "unknown product" (a JSON answer from the new
// handler) apart from an old agent's bare 404 for an unknown route.
func looksLikeReleaseCodeError(raw []byte) bool {
	var probe struct {
		Code string `json:"code"`
	}
	return json.Unmarshal(raw, &probe) == nil && strings.HasPrefix(probe.Code, "release_code.")
}

// CheckStagedCode refuses a staged code the server would reject anyway, so a
// stale build fails in seconds instead of after uploading every artifact.
func CheckStagedCode(stagedCode int64, state *ReleaseCodeState, allowBackfill bool) error {
	if state == nil || allowBackfill || stagedCode > state.HighestPublished {
		return nil
	}
	return fmt.Errorf("release code %d is not greater than the highest published code %d for %s; claim a fresh one with 'relkit release-code claim' (ADR 0025), or pass --allow-backfill if you are deliberately backfilling",
		stagedCode, state.HighestPublished, state.Product)
}
