package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/agentpublish"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/resolution"
)

// cmdRelease implements `relkit release`: the publish-side gate for a vNext
// ArtifactSet that has already been staged. ADR 0019 §1 retired the legacy
// release-policy path (relkit.json control plane, releasegate reconcile,
// Python install/upgrade/ci-release faces); the signed resolution plus
// `relkit.project.json` is the only accepted input shape.
//
// Execute remains CI-only: it requires RELKIT_RELEASE_VIA_CI=1 and the
// Operator secret referenced by the signed profile.
func cmdRelease(args []string) error {
	execute := false
	asJSON := false
	allowBackfill := false
	projectPath := defaultProjectPath
	resolutionPath := selectedResolutionPath()
	version := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--execute":
			execute = true
		case "--json":
			asJSON = true
		case "--allow-backfill":
			allowBackfill = true
		case "--project":
			i++
			projectPath = mustValue(args, i, "--project")
		case "--resolution":
			i++
			resolutionPath = mustValue(args, i, "--resolution")
		case "--version":
			i++
			version = mustValue(args, i, "--version")
		default:
			return fmt.Errorf("unknown flag %q for release", args[i])
		}
	}
	return releaseVNext(projectPath, resolutionPath, version, execute, allowBackfill, asJSON)
}

func releaseVNext(
	projectPath string,
	resolutionPath string,
	version string,
	execute bool,
	allowBackfill bool,
	asJSON bool,
) error {
	if version == "" {
		return fmt.Errorf("vNext release requires --version")
	}
	if execute && os.Getenv("RELKIT_RELEASE_VIA_CI") != "1" {
		return fmt.Errorf("release --execute requires RELKIT_RELEASE_VIA_CI=1")
	}
	productDocument, err := project.Load(projectPath)
	if err != nil {
		return err
	}
	resolved, err := resolution.Load(resolutionPath)
	if err != nil {
		return err
	}
	bundlePath := filepath.Join(filepath.Dir(resolutionPath), "resolution-bundle.json")
	bundle, err := resolution.LoadBundle(bundlePath)
	if err != nil {
		return err
	}
	verified, err := bundle.Verify(resolution.BundlePolicy{
		DistributionRootFingerprint: productDocument.TrustFingerprint,
		CandidateRootFingerprint:    os.Getenv("RELKIT_CANDIDATE_ROOT_FINGERPRINT"),
		OperatorDeploymentID:        resolved.Operator.DeploymentID,
		Identity:                    resolved.Identity,
	})
	if err != nil {
		return fmt.Errorf("resolution bundle: %w", err)
	}
	if verified.Digest != resolved.Digest {
		return fmt.Errorf("resolution bundle digest does not match %s", resolutionPath)
	}
	if !hasPublishContract(resolved.Contracts) {
		return fmt.Errorf("selected distribution does not provide the publishProtocol/agent contract")
	}
	agentURL, err := vNextAgentTransportURL(bundle.Operator.Endpoints["agent"], execute)
	if err != nil {
		return err
	}
	secretRef := bundle.Operator.SecretRefs["agentBearer"]
	if secretRef == "" {
		secretRef = bundle.Operator.SecretRefs["upload"]
	}
	token, err := operatorSecret(secretRef, execute)
	if err != nil {
		return err
	}
	root := filepath.Dir(projectPath)
	if root == "" {
		root = "."
	}
	result, err := agentpublish.Run(context.Background(), agentpublish.Options{
		Root: root, Product: productDocument.Product, Version: version,
		AgentURL: agentURL, Token: token, Execute: execute, AllowBackfill: allowBackfill,
		Log: func(line string) {
			if !asJSON {
				fmt.Fprintln(os.Stderr, line)
			}
		},
	})
	if err != nil {
		return err
	}
	if asJSON {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	if !execute {
		fmt.Printf("release plan: %s/%s via signed Operator %s\n",
			productDocument.Product, version, bundle.Operator.DeploymentID)
		return nil
	}
	fmt.Printf("released %s/%s resolution=%s staged=%s\n",
		productDocument.Product, version, resolved.Digest, result.StagedSHA256)
	return nil
}

func hasPublishContract(values []contracts.Capability) bool {
	for _, value := range values {
		if value.Domain == contracts.DomainPublishProtocol && value.Peer == "agent" &&
			value.Min <= 3 && value.Max >= 3 {
			return true
		}
	}
	return false
}

// vNextAgentTransportURL resolves the agent endpoint used by an execute-mode
// release. The signed Operator endpoint stays the default authority. In CI the
// transport may be overridden by RELKIT_AGENT_TRANSPORT_URL because a build
// network can reach the same publish service through a different scheme, port,
// or host name than the one baked into the signed profile.
//
// The override is a plain explicit endpoint configuration, not a topology
// assertion: it is never compared against the signed endpoint's host or IP, so
// the same configuration works whether the service instance happens to run on
// the same machine or a different one. What is still enforced is configuration
// hygiene (absolute http/https URL, no credentials, no query, no fragment) and
// the CI gate (the variable is only meaningful when RELKIT_RELEASE_VIA_CI=1).
func vNextAgentTransportURL(signedURL string, execute bool) (string, error) {
	signedURL = strings.TrimSpace(signedURL)
	if !execute {
		return signedURL, nil
	}
	override := strings.TrimSpace(os.Getenv("RELKIT_AGENT_TRANSPORT_URL"))
	if override == "" {
		return signedURL, nil
	}
	if os.Getenv("RELKIT_RELEASE_VIA_CI") != "1" {
		return "", fmt.Errorf("RELKIT_AGENT_TRANSPORT_URL requires RELKIT_RELEASE_VIA_CI=1")
	}
	parsed, err := url.Parse(override)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("RELKIT_AGENT_TRANSPORT_URL must be an absolute HTTP(S) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("RELKIT_AGENT_TRANSPORT_URL must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("RELKIT_AGENT_TRANSPORT_URL must not contain credentials, query, or fragment")
	}
	return override, nil
}

func operatorSecret(reference string, required bool) (string, error) {
	if reference == "" {
		if required {
			return "", fmt.Errorf("signed Operator Profile has no agentBearer secret reference")
		}
		return "", nil
	}
	name := ""
	switch {
	case strings.HasPrefix(reference, "env://"):
		name = strings.TrimPrefix(reference, "env://")
	case strings.HasPrefix(reference, "environment://"):
		name = strings.TrimPrefix(reference, "environment://")
	default:
		if required {
			return "", fmt.Errorf("unsupported Operator secret reference %q", reference)
		}
		return "", nil
	}
	if name == "" || strings.ContainsAny(name, " \t\r\n/\\") {
		return "", fmt.Errorf("invalid Operator environment secret reference")
	}
	value := strings.TrimSpace(os.Getenv(name))
	if required && value == "" {
		return "", fmt.Errorf("Operator credential %s is unavailable", name)
	}
	return value, nil
}
