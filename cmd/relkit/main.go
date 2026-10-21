package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/buildver"
	"github.com/shichao402/relkit/internal/jsonio"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/publishproto"
	"github.com/shichao402/relkit/internal/verify"
	vnextbootstrap "github.com/shichao402/relkit/internal/vnext/bootstrap"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/resolution"
	"github.com/shichao402/relkit/internal/vnext/trust"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Overridden at release build time via -ldflags "-X main.version=...".
// Without ldflags (module channel: go run / go install @vX) this resolves
// from debug.ReadBuildInfo; see internal/buildver.
var version = buildver.Placeholder

func main() {
	version = buildver.Resolve(version)
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) (code int) {
	publishproto.PublisherVersion = version
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		switch value := recovered.(type) {
		case error:
			code = fail(value)
		case string:
			code = fail(fmt.Errorf("%s", value))
		default:
			panic(recovered)
		}
	}()

	showVersion, args, err := stripGlobalFlags(argv)
	if err != nil {
		return fail(err)
	}
	if showVersion {
		fmt.Println("relkit " + version)
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage())
		return 2
	}

	command := args[0]
	rest := args[1:]
	switch command {
	case "resolve":
		err = cmdResolve(rest)
	case "doctor":
		err = cmdDoctor(rest)
	case "materialize":
		err = cmdMaterialize(rest)
	case "wrapper":
		err = cmdWrapper(rest)
	case "artifacts":
		err = cmdArtifacts(rest)
	case "payload":
		err = cmdPayload(rest)
	case "stage":
		err = cmdStage(rest)
	case "inspect":
		err = cmdInspect(rest)
	case "verify":
		err = cmdVerify(rest)
	case "release":
		err = cmdRelease(rest)
	case "build":
		err = cmdBuild(rest)
	default:
		err = fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		return fail(err)
	}
	return 0
}

// cmdStage stages a vNext release from an immutable ArtifactSet. ADR 0019 §1
// retired the release-policy/relkit.json staging face: --artifact-set is the
// only accepted input shape.
func cmdStage(args []string) error {
	return cmdStageVNext(args)
}

// cmdInspect prints one resolved document. Without arguments (or with --json)
// it reports the immutable resolution selected for this invocation; --file
// prints a staged/derived protobuf document for diagnostics.
func cmdInspect(args []string) error {
	fileArg := ""
	resolutionArg := ""
	asJSON := false
	raw := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--file":
			i++
			fileArg = mustValue(args, i, "--file")
		case arg == "--resolution":
			i++
			resolutionArg = mustValue(args, i, "--resolution")
		case arg == "--json":
			asJSON = true
		case arg == "--raw":
			raw = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}

	if fileArg == "" {
		if resolutionArg == "" {
			resolutionArg = selectedResolutionPath()
		}
		_ = asJSON
		return inspectVNextResolution(resolutionArg)
	}

	var (
		document map[string]any
		message  proto.Message
	)
	if strings.EqualFold(filepath.Ext(fileArg), ".json") {
		if err := jsonio.LoadPathLenient(fileArg, &document); err != nil {
			return err
		}
	} else {
		loaded, err := loadProtoDocument(fileArg)
		if err != nil {
			return err
		}
		message = loaded
	}

	if document != nil {
		data, err := jsonio.MarshalPretty(document)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(data)
		return err
	}

	if !raw {
		if env, ok := message.(*model.Envelope); ok {
			fmt.Fprintln(os.Stderr, "# envelope signatures are not re-verified by inspect; use `relkit verify released` for trust")
			unsealed, err := unsealForInspection(env)
			if err != nil {
				return err
			}
			message = unsealed
		}
	}

	data, err := marshalProtoJSON(message)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}

// cmdVerify dispatches `relkit verify released`. ADR 0019 §1 retired the
// backend-crawl verifier that read relkit.json: released verification always
// runs against an immutable resolution plus a signed offline bundle.
func cmdVerify(args []string) error {
	if len(args) == 0 || args[0] != "released" {
		return fmt.Errorf("usage: relkit verify released --version <v> --channel <dev|stable> --resolution <path>")
	}
	return cmdVerifyReleased(args[1:])
}

func cmdVerifyReleased(args []string) error {
	options := verify.ReleasedOptions{}
	resolutionPath := strings.TrimSpace(os.Getenv("RELKIT_RESOLUTION"))
	projectPath := defaultProjectPath
	consumerURL := ""
	directoryURL := ""
	var productRootPaths []string
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--channel":
			i++
			options.Channel = mustValue(args, i, "--channel")
		case arg == "--to":
			i++
			options.To = append(options.To, mustValue(args, i, "--to"))
		case arg == "--deep":
			options.Deep = true
		case arg == "--version" || arg == "--expect-version":
			i++
			options.Version = mustValue(args, i, arg)
		case arg == "--resolution":
			i++
			resolutionPath = mustValue(args, i, "--resolution")
		case arg == "--project":
			i++
			projectPath = mustValue(args, i, "--project")
		case arg == "--product-root":
			i++
			productRootPaths = append(productRootPaths, mustValue(args, i, "--product-root"))
		case arg == "--consumer":
			i++
			consumerURL = mustValue(args, i, "--consumer")
		case arg == "--directory":
			i++
			directoryURL = mustValue(args, i, "--directory")
		case arg == "--resolution-digest":
			i++
			options.ResolutionDigest = mustValue(args, i, "--resolution-digest")
		case arg == "--operator":
			i++
			options.OperatorDeploymentID = mustValue(args, i, "--operator")
		case arg == "--min-index-sequence":
			i++
			value, err := strconv.ParseInt(mustValue(args, i, "--min-index-sequence"), 10, 64)
			if err != nil {
				return fmt.Errorf("--min-index-sequence: %w", err)
			}
			options.MinimumIndexSequence = value
		case arg == "--min-directory-sequence":
			i++
			value, err := strconv.ParseInt(mustValue(args, i, "--min-directory-sequence"), 10, 64)
			if err != nil {
				return fmt.Errorf("--min-directory-sequence: %w", err)
			}
			options.MinimumDirectorySequence = value
		default:
			return fmt.Errorf("unknown flag %q", arg)
		}
	}
	if resolutionPath == "" {
		return fmt.Errorf("verify released requires --resolution or RELKIT_RESOLUTION")
	}
	resolved, err := resolution.Load(resolutionPath)
	if err != nil {
		return err
	}
	if options.ResolutionDigest != "" && options.ResolutionDigest != resolved.Digest {
		return fmt.Errorf("--resolution-digest does not match %s", resolutionPath)
	}
	if options.OperatorDeploymentID != "" && options.OperatorDeploymentID != resolved.Operator.DeploymentID {
		return fmt.Errorf("--operator does not match %s", resolutionPath)
	}
	options.ResolutionDigest = resolved.Digest
	options.OperatorDeploymentID = resolved.Operator.DeploymentID
	if options.Version == "" {
		return fmt.Errorf("verify released requires --version")
	}
	return verifyReleasedVNext(projectPath, resolutionPath, consumerURL, directoryURL, productRootPaths, options)
}

func verifyReleasedVNext(
	projectPath, resolutionPath, consumerURL, directoryURL string,
	productRootPaths []string,
	options verify.ReleasedOptions,
) error {
	if options.Channel == "" {
		return fmt.Errorf("vNext verify released requires --channel")
	}
	productDocument, err := project.Load(projectPath)
	if err != nil {
		return err
	}
	resolved, err := resolution.Load(resolutionPath)
	if err != nil {
		return err
	}
	bundle, err := resolution.LoadBundle(filepath.Join(filepath.Dir(resolutionPath), "resolution-bundle.json"))
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
		return err
	}
	if verified.Digest != resolved.Digest {
		return fmt.Errorf("resolution bundle does not match immutable resolution")
	}
	if len(productRootPaths) == 0 {
		if value := os.Getenv("RELKIT_PRODUCT_ROOT"); value != "" {
			productRootPaths = filepath.SplitList(value)
		}
	}
	if len(productRootPaths) == 0 {
		path, pathErr := vnextbootstrap.DefaultPath(productDocument.OperatorRef)
		if pathErr != nil {
			return pathErr
		}
		mapping, loadErr := vnextbootstrap.Load(path)
		if loadErr != nil {
			return loadErr
		}
		productRootPaths = releaseProductRootPaths(mapping, productDocument)
	}
	if len(productRootPaths) == 0 {
		return fmt.Errorf("no Product RUP Root was provisioned for %s", productDocument.Product)
	}
	productRoots, err := loadDistributionKeys(productRootPaths)
	if err != nil {
		return err
	}
	if publication := productDocument.RuntimeProfilePublication; publication != nil {
		matches := false
		for _, key := range productRoots {
			fingerprint, fingerprintErr := trust.Fingerprint(key)
			if fingerprintErr != nil {
				return fingerprintErr
			}
			if fingerprint == publication.RootFingerprint {
				matches = true
			}
		}
		if !matches {
			return fmt.Errorf("provisioned product roots do not match runtimeProfilePublication.rootFingerprint")
		}
	}
	if consumerURL == "" {
		consumerURL = bundle.Operator.Endpoints["consumer"]
	}
	if directoryURL == "" {
		directoryURL = bundle.Operator.Endpoints["directory"]
	}
	findings, err := verify.RunReleasedHTTP(context.Background(), verify.ReleasedHTTPOptions{
		Product: productDocument.Product, Channel: options.Channel, Version: options.Version,
		ConsumerBaseURL: consumerURL, DirectoryURL: directoryURL,
		TrustedProductRoots: productRoots, ResolutionDigest: resolved.Digest,
		OperatorDeploymentID: resolved.Operator.DeploymentID,
		ResolutionIdentity:   string(resolved.Identity),
	})
	if err != nil {
		return err
	}
	for _, note := range findings.Notes {
		fmt.Println("NOTE: " + note)
	}
	for _, warning := range findings.Warnings {
		fmt.Println("WARN: " + warning)
	}
	for _, finding := range findings.Errors {
		fmt.Fprintln(os.Stderr, "ERROR: "+finding)
	}
	if !findings.OK() {
		return exitCodeError{code: 1}
	}
	fmt.Printf("released verification passed for %s at resolution %s\n", options.Version, resolved.Digest)
	return nil
}

func releaseProductRootPaths(mapping *vnextbootstrap.Document, productDocument *project.Document) []string {
	if roots := mapping.ProductRoots[productDocument.Product]; len(roots) > 0 {
		return append([]string(nil), roots...)
	}
	if productDocument.RuntimeProfilePublication != nil {
		if runtimeProfile, ok := mapping.RuntimeProfiles[productDocument.Product]; ok {
			return append([]string(nil), runtimeProfile.Roots...)
		}
	}
	return nil
}

func unsealForInspection(env *model.Envelope) (proto.Message, error) {
	index, err := rupv2.UnmarshalIndex(env.Payload)
	if err != nil {
		return nil, err
	}
	return index, nil
}

func loadProtoDocument(path string) (proto.Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if env, err := rupv2.UnmarshalEnvelope(data); err == nil && env.Schema == model.SchemaEnvelope {
		return env, nil
	}
	if index, err := rupv2.UnmarshalIndex(data); err == nil && index.Schema == model.SchemaIndex {
		return index, nil
	}
	if manifest, err := rupv2.UnmarshalManifest(data); err == nil && manifest.Schema == model.SchemaManifest {
		return manifest, nil
	}
	if staged, err := rupv2.UnmarshalStaged(data); err == nil && staged.Schema == model.SchemaStaged {
		return staged, nil
	}
	if publicKey, err := rupv2.UnmarshalPublicKey(data); err == nil && publicKey.Schema == model.SchemaPublicKey {
		return publicKey, nil
	}
	if privateKey, err := rupv2.UnmarshalPrivateKey(data); err == nil && privateKey.Schema == model.SchemaPrivateKey {
		return privateKey, nil
	}
	return nil, fmt.Errorf("could not recognize protobuf document: %s", path)
}

func marshalProtoJSON(message proto.Message) ([]byte, error) {
	if message == nil {
		return nil, fmt.Errorf("no protobuf message to print")
	}
	data, err := protojson.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}.Marshal(message)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func stripGlobalFlags(args []string) (bool, []string, error) {
	showVersion := false
	var remaining []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--version":
			// Only a leading `relkit --version` is global; subcommand
			// `--version <product version>` flags stay in argv.
			if len(remaining) == 0 {
				showVersion = true
			} else {
				remaining = append(remaining, arg)
			}
		default:
			remaining = append(remaining, arg)
		}
	}
	return showVersion, remaining, nil
}

func mustValue(args []string, index int, flagName string) string {
	if index >= len(args) {
		panic(fmt.Errorf("%s requires a value", flagName))
	}
	return args[index]
}

type exitCodeError struct {
	code int
}

func (e exitCodeError) Error() string {
	return ""
}

func fail(err error) int {
	var exitErr exitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.code
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	return 2
}

func usage() string {
	return strings.TrimSpace(`
relkit ` + version + `

Commands:
  resolve
  doctor
  materialize
  wrapper
  artifacts
  payload
  stage
  inspect
  verify
  release
  build
`)
}
