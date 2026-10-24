package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/runtimebootstrap"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/tools"
)

// cmdBootstrap answers the one question a host build cannot answer on its own:
// which server root this application trusts, and what that server's signed
// Runtime Profile says.
//
// The document it writes is relkit.runtime-bootstrap/2, embedded verbatim by
// the host build and forwarded to the engine through ClientProfile.bootstrap
// (requirements 3.8, 4.4). A host never parses it: the engine owns the format.
//
// Everything it needs already lives in relkit.project.json, so a host keeps
// holding exactly four things (requirement 12.1) and the server stays the single
// source of truth for which Profile is current. That is why this is a build
// input produced here rather than a document the host assembles: assembling it
// would require the host to know the format.
func cmdBootstrap(args []string) error {
	if len(args) == 0 || args[0] != "build" {
		return fmt.Errorf("usage: relkit bootstrap build [--project <relkit.project.json>] [--out <bootstrap.json>] [--profile-url <url>] [--timeout <seconds>]")
	}
	projectPath := defaultProjectPath
	outputPath := ""
	profileURL := ""
	timeout := 30 * time.Second
	for i := 1; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--project":
			i++
			projectPath = mustValue(args, i, "--project")
		case arg == "--out":
			i++
			outputPath = mustValue(args, i, "--out")
		case arg == "--profile-url":
			i++
			profileURL = mustValue(args, i, "--profile-url")
		case arg == "--timeout":
			i++
			seconds, err := strconv.Atoi(mustValue(args, i, "--timeout"))
			if err != nil {
				return fmt.Errorf("--timeout: %w", err)
			}
			if seconds <= 0 {
				return fmt.Errorf("--timeout must be positive")
			}
			timeout = time.Duration(seconds) * time.Second
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}

	productDocument, err := project.Load(projectPath)
	if err != nil {
		return err
	}
	server, err := productDocument.ServerURL()
	if err != nil {
		return err
	}
	root, err := productDocument.RootKey()
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: timeout}
	rootKeyID, err := serverRootKeyID(context.Background(), client, server, root)
	if err != nil {
		return err
	}
	profile, err := fetchSignedProfile(context.Background(), client, server, productDocument.Product, rootKeyID, root, profileURL)
	if err != nil {
		return err
	}
	value, err := runtimebootstrap.New(rootKeyID, root, productDocument.Product, *profile)
	if err != nil {
		return err
	}
	raw, err := canonical.Bytes(*value)
	if err != nil {
		return err
	}

	if outputPath == "" {
		outputPath = filepath.Join(filepath.Dir(projectPath), ".relkit", "runtime-bootstrap.json")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("built Runtime Bootstrap for %s (profile %s)\n", productDocument.Product, profile.Digest)
	fmt.Println("wrote " + outputPath)
	return nil
}

// serverRootKeyID reports the key id the server signs with.
//
// It reads the signed tool manifest rather than the Profile, because a
// signature's self-declared keyId proves nothing: trusting it would let any
// document name the key it happens to be signed with. The manifest is the one
// document that states the id and is itself verifiable against the root key the
// project already holds, so it is the authority for the label.
func serverRootKeyID(ctx context.Context, client *http.Client, server string, root ed25519.PublicKey) (string, error) {
	manifestURL := strings.TrimRight(server, "/") + "/v1/tools/current/manifest.json"
	raw, err := getBytes(ctx, client, manifestURL)
	if err != nil {
		return "", fmt.Errorf("read tool manifest %s: %w", manifestURL, err)
	}
	var manifest tools.Document
	if err := document.Decode(raw, &manifest); err != nil {
		return "", fmt.Errorf("tool manifest %s is not valid JSON: %w", manifestURL, err)
	}
	keyID := strings.TrimSpace(manifest.RootKeyID)
	if keyID == "" {
		return "", fmt.Errorf("tool manifest %s declares no rootKeyId", manifestURL)
	}
	// Verify before trusting the id: the manifest must be signed by the same
	// key the project pins, otherwise its stated id means nothing.
	if err := manifest.Verify(map[string]ed25519.PublicKey{keyID: root}); err != nil {
		return "", fmt.Errorf("tool manifest %s is not signed by the project root key: %w", manifestURL, err)
	}
	return keyID, nil
}

// fetchSignedProfile reads the Runtime Profile the server currently serves.
//
// The signed directory is the only index of what the server points at, so the
// Profile is discovered from it rather than guessed: the directory carries the
// digest and URLs of the Profile it was published with (requirement 10.5).
func fetchSignedProfile(
	ctx context.Context,
	client *http.Client,
	server, product, rootKeyID string,
	root ed25519.PublicKey,
	explicitURL string,
) (*runtimeprofile.Document, error) {
	url := explicitURL
	if url == "" {
		discovered, err := discoverProfileURL(ctx, client, server, product, rootKeyID, root)
		if err != nil {
			return nil, err
		}
		url = discovered
	}

	raw, err := getBytes(ctx, client, url)
	if err != nil {
		return nil, err
	}
	var profile runtimeprofile.Document
	if err := document.Decode(raw, &profile); err != nil {
		return nil, fmt.Errorf("runtime profile %s is not valid JSON: %w", url, err)
	}
	// The key id came from the verified tool manifest; the Profile only has to
	// prove it was signed by that id with the pinned key bytes.
	if err := profile.Verify(map[string]ed25519.PublicKey{rootKeyID: root}); err != nil {
		return nil, fmt.Errorf("runtime profile %s is not signed by the server root %q: %w", url, rootKeyID, err)
	}
	if profile.Product != product {
		return nil, fmt.Errorf("runtime profile belongs to product %q, expected %q", profile.Product, product)
	}
	return &profile, nil
}

// discoverProfileURL reads the signed directory and returns the Profile URL it
// points at. A server that never published a Profile cannot supply a bootstrap,
// and that is reported plainly instead of papered over.
func discoverProfileURL(
	ctx context.Context,
	client *http.Client,
	server, product, rootKeyID string,
	root ed25519.PublicKey,
) (string, error) {
	directoryURL := strings.TrimRight(server, "/") + "/" + strings.TrimLeft(model.DirectoryKey(product), "/")
	raw, err := getBytes(ctx, client, directoryURL)
	if err != nil {
		return "", fmt.Errorf("read signed directory %s: %w", directoryURL, err)
	}
	envelopeValue, err := rupv2.UnmarshalEnvelope(raw)
	if err != nil {
		return "", fmt.Errorf("signed directory %s is not valid protobuf: %w", directoryURL, err)
	}
	directory, err := envelope.OpenDirectoryEnvelope(envelopeValue, map[string]ed25519.PublicKey{rootKeyID: root})
	if err != nil {
		return "", fmt.Errorf("signed directory %s is not trusted: %w", directoryURL, err)
	}
	if directory.Product != product {
		return "", fmt.Errorf("signed directory belongs to product %q, expected %q", directory.Product, product)
	}
	reference := directory.GetRuntimeProfile()
	if reference == nil || len(reference.Urls) == 0 {
		return "", fmt.Errorf(
			"server %s serves no Runtime Profile for %s; publish one first (POST /v1/runtime-profile)",
			server, product,
		)
	}
	return reference.Urls[0], nil
}

func getBytes(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("read %s: HTTP %d", url, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	return body, nil
}
