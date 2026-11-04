// Package agentpublish uploads an immutable staged tree and asks a Relkit
// Agent to commit it. Product configuration and signing keys remain on the
// Operator side; callers only supply the signed-resolution deployment facts.
package agentpublish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/casput"
	"github.com/shichao402/relkit/internal/publishproto"
	"github.com/shichao402/relkit/internal/stage"
)

type Options struct {
	Root          string
	Product       string
	Version       string
	AgentURL      string
	Token         string
	Execute       bool
	AllowBackfill bool
	Client        *http.Client
	Log           func(string)
}

type Result struct {
	Product       string          `json:"product"`
	Version       string          `json:"version"`
	AgentURL      string          `json:"agentUrl"`
	Execute       bool            `json:"execute"`
	Uploaded      int             `json:"uploaded,omitempty"`
	Skipped       int             `json:"skipped,omitempty"`
	StagedSHA256  string          `json:"stagedSha256,omitempty"`
	StagedBytes   int64           `json:"stagedBytes,omitempty"`
	AgentResponse json.RawMessage `json:"agentResponse,omitempty"`
}

func Run(ctx context.Context, options Options) (*Result, error) {
	if options.Root == "" {
		options.Root = "."
	}
	if options.Product == "" || options.Version == "" {
		return nil, fmt.Errorf("product and version are required")
	}
	if strings.TrimSpace(options.AgentURL) == "" {
		return nil, fmt.Errorf("signed Operator Profile has no agent endpoint")
	}
	staged := filepath.Join(options.Root, ".relkit", "cache", "staged", options.Version)
	if info, err := os.Stat(staged); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("no staged tree for %s: %s", options.Version, filepath.ToSlash(staged))
	}
	result := &Result{
		Product: options.Product, Version: options.Version,
		AgentURL: options.AgentURL, Execute: options.Execute,
	}
	if !options.Execute {
		return result, nil
	}
	if strings.TrimSpace(options.Token) == "" {
		return nil, fmt.Errorf("Operator agent credential is unavailable")
	}
	logger := options.Log
	if logger == nil {
		logger = func(string) {}
	}
	if err := preflightStagedCode(ctx, options, logger); err != nil {
		return nil, err
	}
	put, err := casput.Put(ctx, casput.Options{
		Root: options.Root, Product: options.Product, Version: options.Version,
		URL: options.AgentURL, Token: options.Token, Log: logger,
	})
	if err != nil {
		return nil, fmt.Errorf("upload staged tree: %w", err)
	}
	if put.StagedSHA256 == "" {
		return nil, fmt.Errorf("agent upload did not return staged SHA-256")
	}
	result.Uploaded = put.Uploaded
	result.Skipped = put.Skipped
	result.StagedSHA256 = put.StagedSHA256
	result.StagedBytes = put.StagedBytes

	payload, err := json.Marshal(map[string]any{
		"product":        options.Product,
		"version":        options.Version,
		"stagedSha256":   put.StagedSHA256,
		"idempotencyKey": options.Product + "/" + options.Version + "/" + put.StagedSHA256,
		"allowBackfill":  options.AllowBackfill,
	})
	if err != nil {
		return nil, err
	}
	endpoint := normalizeEndpoint(options.AgentURL, "publish")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+options.Token)
	request.Header.Set("Content-Type", "application/json")
	publishproto.Apply(request.Header)
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("agent publish %s: %w", endpoint, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode != http.StatusOK {
		if protocolErr := publishproto.Explain(response.StatusCode, body); protocolErr != nil {
			return nil, protocolErr
		}
		return nil, fmt.Errorf("agent publish returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	result.AgentResponse = append(json.RawMessage(nil), body...)
	return result, nil
}

// preflightStagedCode compares the staged release code with what the server has
// already published before any byte is uploaded. An agent that predates ADR
// 0025 cannot answer; then the publish-time check is the only guard, as before.
func preflightStagedCode(ctx context.Context, options Options, log func(string)) error {
	staged, err := stage.LoadStaged(options.Root, options.Version)
	if err != nil {
		return err
	}
	state, err := ReadReleaseCode(ctx, ReleaseCodeOptions{
		Product: options.Product, AgentURL: options.AgentURL, Token: options.Token, Client: options.Client,
	})
	if errors.Is(err, ErrReleaseCodeUnsupported) {
		log("release-code preflight skipped: " + err.Error())
		return nil
	}
	if err != nil {
		return fmt.Errorf("release-code preflight: %w", err)
	}
	return CheckStagedCode(staged.GetCode(), state, options.AllowBackfill)
}

func normalizeEndpoint(baseURL, action string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base + "/" + action
}
