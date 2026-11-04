package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/shichao402/relkit/internal/agentpublish"
	"github.com/shichao402/relkit/internal/vnext/project"
)

// cmdReleaseCode implements `relkit release-code claim|show` (ADR 0025).
//
// The release code orders a product's releases and names its installs on the
// client, so it is a server fact. A host claims one before it builds and feeds
// it to its producer; it no longer derives one from git history or CI counters.
//
// `claim` prints just the number on stdout so CI can capture it directly:
//
//	code="$(relkit release-code claim)"
//
// Inputs mirror `relkit release`: the server comes from relkit.project.json and
// the token from the credential manifest by keyId.
func cmdReleaseCode(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: relkit release-code claim|show [--project <p>] [--custody <m>] [--token-key <id>] [--json]")
	}
	action := args[0]
	if action != "claim" && action != "show" {
		return fmt.Errorf("unknown release-code action %q (want claim or show)", action)
	}
	asJSON := false
	projectPath := defaultProjectPath
	manifestPath := ""
	tokenKeyID := ""
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--json":
			asJSON = true
		case "--project":
			i++
			projectPath = mustValue(rest, i, "--project")
		case "--custody":
			i++
			manifestPath = mustValue(rest, i, "--custody")
		case "--token-key":
			i++
			tokenKeyID = mustValue(rest, i, "--token-key")
		default:
			return fmt.Errorf("unknown flag %q for release-code %s", rest[i], action)
		}
	}
	productDocument, err := project.Load(projectPath)
	if err != nil {
		return err
	}
	serverURL, err := productDocument.ServerURL()
	if err != nil {
		return err
	}
	// The same transport rule as an execute-mode release: the declared server,
	// or RELKIT_AGENT_TRANSPORT_URL when CI sets it explicitly.
	agentURL, err := vNextAgentTransportURL(serverURL, true)
	if err != nil {
		return err
	}
	token, err := releaseToken(manifestPath, tokenKeyID, true)
	if err != nil {
		return err
	}
	options := agentpublish.ReleaseCodeOptions{Product: productDocument.Product, AgentURL: agentURL, Token: token}
	var state *agentpublish.ReleaseCodeState
	if action == "claim" {
		state, err = agentpublish.ClaimReleaseCode(context.Background(), options)
	} else {
		state, err = agentpublish.ReadReleaseCode(context.Background(), options)
	}
	if err != nil {
		return err
	}
	if asJSON {
		data, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	if action == "claim" {
		fmt.Fprintf(os.Stderr, "claimed release code %d for %s (highest published %d)\n", state.Code, state.Product, state.HighestPublished)
		fmt.Println(state.Code)
		return nil
	}
	fmt.Printf("product=%s lastAllocated=%d highestPublished=%d\n", state.Product, state.LastAllocated, state.HighestPublished)
	return nil
}
