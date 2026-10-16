package verify

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/model"
)

type ReleasedHTTPOptions struct {
	Product              string
	Channel              string
	Version              string
	ConsumerBaseURL      string
	DirectoryURL         string
	TrustedProductRoots  map[string]ed25519.PublicKey
	ResolutionDigest     string
	OperatorDeploymentID string
	ResolutionIdentity   string
	Client               *http.Client
}

// RunReleasedHTTP independently reads consumer-visible RUP documents. It does
// not use the Agent write response or product-repository backend credentials.
func RunReleasedHTTP(ctx context.Context, options ReleasedHTTPOptions) (*Findings, error) {
	if options.Product == "" || options.Channel == "" || options.Version == "" {
		return nil, fmt.Errorf("product, channel, and version are required")
	}
	if options.ConsumerBaseURL == "" {
		return nil, fmt.Errorf("signed Operator Profile has no consumer endpoint")
	}
	if len(options.TrustedProductRoots) == 0 {
		return nil, fmt.Errorf("no trusted product RUP root was provisioned")
	}
	client := options.Client
	if client == nil {
		client = http.DefaultClient
	}
	findings := &Findings{}
	indexRaw, err := fetchConsumer(ctx, client, objectURL(options.ConsumerBaseURL, model.IndexKey(options.Product, options.Channel)))
	if err != nil {
		return nil, err
	}
	indexEnvelope, err := rupv2.UnmarshalEnvelope(indexRaw)
	if err != nil {
		return nil, fmt.Errorf("consumer index is invalid protobuf: %w", err)
	}
	index, err := envelope.OpenEnvelope(indexEnvelope, options.TrustedProductRoots)
	if err != nil {
		return nil, fmt.Errorf("consumer index is untrusted: %w", err)
	}
	var selected *model.VersionNode
	for _, node := range index.Versions {
		if node != nil && node.Version == options.Version {
			selected = node
			break
		}
	}
	if selected == nil {
		findings.Error(fmt.Sprintf("version %s is absent from consumer index", options.Version))
	} else {
		checkHTTPProvenance("index", selected.ResolutionDigest, selected.OperatorDeploymentId, selected.ResolutionIdentity, options, findings)
		manifestRaw, fetchErr := fetchConsumer(ctx, client, objectURL(options.ConsumerBaseURL, model.ManifestKey(options.Product, options.Version)))
		if fetchErr != nil {
			findings.Error(fetchErr.Error())
		} else {
			if selected.Manifest == nil ||
				selected.Manifest.Sha256 != model.Sha256Bytes(manifestRaw) ||
				selected.Manifest.Size != int64(len(manifestRaw)) {
				findings.Error("consumer manifest bytes do not match the signed index reference")
			}
			manifest, parseErr := rupv2.UnmarshalManifest(manifestRaw)
			if parseErr != nil {
				findings.Error(fmt.Sprintf("consumer manifest is invalid: %v", parseErr))
			} else {
				checkHTTPProvenance("manifest", manifest.ResolutionDigest, manifest.OperatorDeploymentId, manifest.ResolutionIdentity, options, findings)
			}
		}
	}

	directoryURL := options.DirectoryURL
	if directoryURL == "" {
		directoryURL = objectURL(options.ConsumerBaseURL, model.DirectoryKey(options.Product))
	}
	directoryRaw, err := fetchConsumer(ctx, client, directoryURL)
	if err != nil {
		findings.Error(err.Error())
		return findings, nil
	}
	directoryEnvelope, err := rupv2.UnmarshalEnvelope(directoryRaw)
	if err != nil {
		findings.Error(fmt.Sprintf("consumer directory is invalid protobuf: %v", err))
		return findings, nil
	}
	directory, err := envelope.OpenDirectoryEnvelope(directoryEnvelope, options.TrustedProductRoots)
	if err != nil {
		findings.Error(fmt.Sprintf("consumer directory is untrusted: %v", err))
		return findings, nil
	}
	if directory.Product != options.Product {
		findings.Error(fmt.Sprintf("consumer directory has product %q", directory.Product))
	}
	return findings, nil
}

func checkHTTPProvenance(
	name, resolutionDigest, operatorID, identity string,
	options ReleasedHTTPOptions,
	findings *Findings,
) {
	if resolutionDigest != options.ResolutionDigest {
		findings.Error(fmt.Sprintf("%s resolution is %q, expected %q", name, resolutionDigest, options.ResolutionDigest))
	}
	if operatorID != options.OperatorDeploymentID {
		findings.Error(fmt.Sprintf("%s Operator is %q, expected %q", name, operatorID, options.OperatorDeploymentID))
	}
	if identity != options.ResolutionIdentity {
		findings.Error(fmt.Sprintf("%s identity is %q, expected %q", name, identity, options.ResolutionIdentity))
	}
}

func fetchConsumer(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read consumer object %s: %w", url, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("read consumer object %s: HTTP %d", url, response.StatusCode)
	}
	return body, nil
}

func objectURL(base, key string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(key, "/")
}
