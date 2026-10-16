package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/publishproto"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
)

type CreateRuntimeProfileOptions struct {
	ProjectPath    string
	EntryURLs      []string
	ProfileVersion int64
	KeyPaths       []string
	Output         string
	Replace        bool
}

type PublishRuntimeProfileOptions struct {
	ProjectPath string
	AgentURL    string
	Token       string
	To          []string
	DryRun      bool
	Output      string
	Replace     bool
	HTTPClient  *http.Client
}

type PublishRuntimeProfileResult struct {
	OK                bool                    `json:"ok"`
	DryRun            bool                    `json:"dryRun"`
	Profile           runtimeprofile.Document `json:"profile"`
	Reference         model.ManifestRef       `json:"reference"`
	DirectorySequence int64                   `json:"directorySequence"`
	Log               []string                `json:"log,omitempty"`
}

func CreateRuntimeProfile(options CreateRuntimeProfileOptions) (*runtimeprofile.Document, error) {
	product, err := project.Load(options.ProjectPath)
	if err != nil {
		return nil, err
	}
	if len(options.KeyPaths) == 0 {
		return nil, fmt.Errorf("at least one product signing key is required")
	}
	signers := make([]envelope.Signer, 0, len(options.KeyPaths))
	seen := map[string]bool{}
	for _, path := range options.KeyPaths {
		keyID, seed, err := loadSigningKey(path)
		if err != nil {
			return nil, err
		}
		if seen[keyID] {
			return nil, fmt.Errorf("duplicate product signing key %q", keyID)
		}
		seen[keyID] = true
		signers = append(signers, envelope.Signer{KeyID: keyID, Seed: seed})
	}
	value, err := runtimeprofile.Create(
		*product,
		options.EntryURLs,
		options.ProfileVersion,
		signers,
	)
	if err != nil {
		return nil, err
	}
	if options.Output == "" {
		return nil, fmt.Errorf("Runtime Profile output path is required")
	}
	if options.Replace {
		err = document.WriteAtomic(options.Output, value)
	} else {
		err = document.WriteNew(options.Output, value)
	}
	if err != nil {
		return nil, err
	}
	return value, nil
}

// PublishRuntimeProfile asks a product-scoped agent to sign and publish the
// first Runtime Profile. The private product key never leaves the agent host.
func PublishRuntimeProfile(ctx context.Context, options PublishRuntimeProfileOptions) (*PublishRuntimeProfileResult, error) {
	product, err := project.Load(options.ProjectPath)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(options.AgentURL) == "" {
		return nil, fmt.Errorf("agent URL is required")
	}
	if strings.TrimSpace(options.Token) == "" {
		return nil, fmt.Errorf("product upload token is required")
	}
	if !options.DryRun && options.Output == "" {
		return nil, fmt.Errorf("Runtime Profile output path is required")
	}
	payload, err := json.Marshal(map[string]any{
		"product": product.Product,
		"project": product,
		"to":      options.To,
		"dryRun":  options.DryRun,
	})
	if err != nil {
		return nil, err
	}
	endpoint := runtimeProfileEndpoint(options.AgentURL)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(options.Token))
	request.Header.Set("Content-Type", "application/json")
	publishproto.Apply(request.Header)
	client := options.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("publish Runtime Profile: %w", err)
	}
	defer response.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if readErr != nil {
		return nil, readErr
	}
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &failure)
		message := strings.TrimSpace(failure.Error)
		if message == "" {
			message = strings.TrimSpace(string(raw))
		}
		if failure.Code != "" {
			return nil, fmt.Errorf("%s: %s", failure.Code, message)
		}
		return nil, fmt.Errorf("agent returned HTTP %d: %s", response.StatusCode, message)
	}
	var result PublishRuntimeProfileResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode Runtime Profile response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("agent returned an unsuccessful Runtime Profile result")
	}
	if err := result.Profile.Validate(); err != nil {
		return nil, fmt.Errorf("agent returned invalid Runtime Profile: %w", err)
	}
	if result.Profile.Product != product.Product {
		return nil, fmt.Errorf("agent returned Runtime Profile for %q, expected %q", result.Profile.Product, product.Product)
	}
	if !options.DryRun {
		if options.Replace {
			err = document.WriteAtomic(options.Output, result.Profile)
		} else {
			err = document.WriteNew(options.Output, result.Profile)
		}
		if err != nil {
			return nil, err
		}
	}
	return &result, nil
}

func runtimeProfileEndpoint(base string) string {
	value := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(value, "/v1") {
		return value + "/runtime-profile"
	}
	return value + "/v1/runtime-profile"
}
