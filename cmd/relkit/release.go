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
	"github.com/shichao402/relkit/internal/custody"
	"github.com/shichao402/relkit/internal/vnext/project"
)

// cmdRelease implements `relkit release`: it publishes an already staged tree
// to the project's agent server.
//
// Inputs are the project declaration and a token, and nothing else
// (requirement 14.9 / M9). There is no resolution, no Operator Profile file,
// and no bundle: the server address comes from `relkit.project.json`, and the
// token comes from the host CI copy of the credential manifest by keyId
// (requirement 15).
//
// Execute remains CI-only: it requires RELKIT_RELEASE_VIA_CI=1.
func cmdRelease(args []string) error {
	execute := false
	asJSON := false
	allowBackfill := false
	projectPath := defaultProjectPath
	manifestPath := ""
	tokenKeyID := ""
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
		case "--custody":
			i++
			manifestPath = mustValue(args, i, "--custody")
		case "--token-key":
			i++
			tokenKeyID = mustValue(args, i, "--token-key")
		case "--version":
			i++
			version = mustValue(args, i, "--version")
		default:
			return fmt.Errorf("unknown flag %q for release", args[i])
		}
	}
	return releaseVNext(releaseOptions{
		projectPath:   projectPath,
		manifestPath:  manifestPath,
		tokenKeyID:    tokenKeyID,
		version:       version,
		execute:       execute,
		allowBackfill: allowBackfill,
		asJSON:        asJSON,
	})
}

type releaseOptions struct {
	projectPath   string
	manifestPath  string
	tokenKeyID    string
	version       string
	execute       bool
	allowBackfill bool
	asJSON        bool
}

func releaseVNext(options releaseOptions) error {
	if options.version == "" {
		return fmt.Errorf("release requires --version")
	}
	if options.execute && os.Getenv("RELKIT_RELEASE_VIA_CI") != "1" {
		return fmt.Errorf("release --execute requires RELKIT_RELEASE_VIA_CI=1")
	}
	productDocument, err := project.Load(options.projectPath)
	if err != nil {
		return err
	}
	serverURL, err := productDocument.ServerURL()
	if err != nil {
		return err
	}
	agentURL, err := vNextAgentTransportURL(serverURL, options.execute)
	if err != nil {
		return err
	}
	token, err := releaseToken(options.manifestPath, options.tokenKeyID, options.execute)
	if err != nil {
		return err
	}
	root := filepath.Dir(options.projectPath)
	if root == "" {
		root = "."
	}
	result, err := agentpublish.Run(context.Background(), agentpublish.Options{
		Root: root, Product: productDocument.Product, Version: options.version,
		AgentURL: agentURL, Token: token, Execute: options.execute,
		AllowBackfill: options.allowBackfill,
		Log: func(line string) {
			if !options.asJSON {
				fmt.Fprintln(os.Stderr, line)
			}
		},
	})
	if err != nil {
		return err
	}
	if options.asJSON {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	if !options.execute {
		fmt.Printf("release plan: %s/%s via %s\n", productDocument.Product, options.version, agentURL)
		return nil
	}
	fmt.Printf("released %s/%s server=%s staged=%s\n",
		productDocument.Product, options.version, agentURL, result.StagedSHA256)
	return nil
}

// releaseToken reads the agent token from the host CI copy of the credential
// manifest. A dry run tolerates a missing token so a plan can be produced on a
// machine that does not hold CI secrets.
func releaseToken(manifestPath, keyID string, required bool) (string, error) {
	if manifestPath == "" {
		manifestPath = strings.TrimSpace(os.Getenv("RELKIT_CUSTODY_MANIFEST"))
	}
	if manifestPath == "" {
		if required {
			return "", fmt.Errorf("release requires --custody <manifest> or RELKIT_CUSTODY_MANIFEST")
		}
		return "", nil
	}
	manifest, err := custody.Load(manifestPath)
	if err != nil {
		if !required {
			return "", nil
		}
		return "", err
	}
	if keyID == "" {
		resolved, resolveErr := singleTokenKeyID(manifest)
		if resolveErr != nil {
			if !required {
				return "", nil
			}
			return "", resolveErr
		}
		keyID = resolved
	}
	token, err := manifest.Token(keyID)
	if err != nil {
		if !required {
			return "", nil
		}
		return "", err
	}
	return token, nil
}

func singleTokenKeyID(manifest *custody.Manifest) (string, error) {
	var found []string
	for _, entry := range manifest.Keys {
		if entry.Kind == custody.KindTokenRef {
			found = append(found, entry.KeyID)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("credential manifest %s registers no token; pass --token-key", manifest.Scope)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("credential manifest registers %d tokens; pass --token-key", len(found))
	}
}

// vNextAgentTransportURL resolves the agent endpoint used by an execute-mode
// release. The project-declared server stays the default authority. In CI the
// transport may be overridden by RELKIT_AGENT_TRANSPORT_URL because a build
// network can reach the same publish service through a different scheme, port,
// or host name than the one written in the project file.
//
// The override is a plain explicit endpoint configuration, not a topology
// assertion: it is never compared against the declared server's host or IP, so
// the same configuration works whether the service instance happens to run on
// the same machine or a different one. What is still enforced is configuration
// hygiene (absolute http/https URL, no credentials, no query, no fragment) and
// the CI gate (the variable is only meaningful when RELKIT_RELEASE_VIA_CI=1).
func vNextAgentTransportURL(declaredURL string, execute bool) (string, error) {
	declaredURL = strings.TrimSpace(declaredURL)
	if !execute {
		return declaredURL, nil
	}
	override := strings.TrimSpace(os.Getenv("RELKIT_AGENT_TRANSPORT_URL"))
	if override == "" {
		return declaredURL, nil
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
