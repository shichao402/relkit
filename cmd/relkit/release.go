package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/agentpublish"
	"github.com/shichao402/relkit/internal/releasegate"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/resolution"
)

// cmdRelease implements `relkit release [--execute]`: the publish-side gate
// for a tree that is already staged. It mirrors hostlib/release.py's
// cmd_release via_ci branch: version reconcile gate (lock drift, staged
// presence), incomplete-steps gate, stale staged cleanup, then agent
// publish (cas-put + POST /publish). Collection (packScript, drops,
// multi-platform aggregation) stays in the product CI's own scripts; this
// command only publishes what they staged.
//
// Execute is CI-only: it requires RELKIT_RELEASE_VIA_CI=1 and
// RELKIT_UPLOAD_TOKEN, mirroring the Python entry.
func cmdRelease(args []string) error {
	execute := false
	asJSON := false
	allowBackfill := false
	projectPath := defaultProjectPath
	resolutionPath := selectedResolutionPath()
	version := ""
	explicitVNext := os.Getenv("RELKIT_RESOLUTION") != ""
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
			explicitVNext = true
		case "--resolution":
			i++
			resolutionPath = mustValue(args, i, "--resolution")
			explicitVNext = true
		case "--version":
			i++
			version = mustValue(args, i, "--version")
			explicitVNext = true
		default:
			return fmt.Errorf("unknown flag %q for release", args[i])
		}
	}
	if !explicitVNext {
		if _, err := os.Stat(projectPath); err == nil {
			explicitVNext = true
		}
	}
	if explicitVNext {
		return releaseVNext(projectPath, resolutionPath, version, execute, allowBackfill, asJSON)
	}
	root := "."

	if execute && os.Getenv("RELKIT_RELEASE_VIA_CI") != "1" {
		return fmt.Errorf("release --execute requires RELKIT_RELEASE_VIA_CI=1 (CI holds the agent token; local shells must not publish)")
	}

	// Version: VERSION.json is the SSOT; the staged tree is keyed by it.
	version, err := releasegate.ReleaseVersionFor(root)
	if err != nil {
		return err
	}

	// The release gate: reconcile slice + incomplete steps.
	report, err := releasegate.Reconcile(root, version)
	if err != nil {
		return err
	}
	if len(report.Drift) > 0 {
		return fmt.Errorf("release refused: unresolved drift\n  %s", strings.Join(report.Drift, "\n  "))
	}
	if len(report.Unconfirmed) > 0 {
		return fmt.Errorf("release refused: cannot confirm remote state\n  %s", strings.Join(report.Unconfirmed, "\n  "))
	}

	viaCI := os.Getenv("RELKIT_RELEASE_VIA_CI") == "1"
	state, err := releasegate.LoadState(root)
	if err != nil {
		return err
	}
	if missing := releasegate.IncompleteSteps(state, viaCI, root); len(missing) > 0 {
		return fmt.Errorf("release refused: incomplete steps: %s", strings.Join(missing, ", "))
	}

	agent := releasegate.AgentBaseURL(root)
	if agent == "" {
		return fmt.Errorf("relkit.json agent.url is required for release; run relkit_host.py agent add (ops surface stays Python during the migration)")
	}
	fmt.Printf("relkit release %s execute=%t\n", version, execute)

	if execute {
		removed, err := releasegate.ClearStaleStagedTrees(root, version)
		if err != nil {
			return err
		}
		if len(removed) > 0 {
			fmt.Printf("removed stale staged caches: %s\n", strings.Join(removed, ", "))
		}
	}
	return releasegate.PublishViaAgent(root, version, execute)
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

// vNextAgentTransportURL preserves the signed Operator endpoint as the
// authority while allowing a CI adapter to carry the same protocol through a
// local tunnel. The credential may only be redirected to a loopback listener:
// arbitrary endpoint overrides would let a compromised build exfiltrate the
// product-scoped Agent token.
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
	parsed, err := url.ParseRequestURI(override)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("RELKIT_AGENT_TRANSPORT_URL must be an absolute HTTP(S) loopback URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("RELKIT_AGENT_TRANSPORT_URL must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("RELKIT_AGENT_TRANSPORT_URL must not contain credentials, query, or fragment")
	}
	host := net.ParseIP(parsed.Hostname())
	if host == nil || !host.IsLoopback() {
		return "", fmt.Errorf("RELKIT_AGENT_TRANSPORT_URL must target a loopback IP")
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
