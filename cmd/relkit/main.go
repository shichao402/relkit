package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/buildver"
	"github.com/shichao402/relkit/internal/jsonio"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/publishproto"
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
	case "artifacts":
		err = cmdArtifacts(rest)
	case "bootstrap":
		err = cmdBootstrap(rest)
	case "payload":
		err = cmdPayload(rest)
	case "stage":
		err = cmdStage(rest)
	case "inspect":
		err = cmdInspect(rest)
	case "release":
		err = cmdRelease(rest)
	case "build":
		err = cmdBuild(rest)
	case "custody":
		err = cmdCustody(rest)
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

// cmdInspect prints one protobuf or JSON document for diagnostics. It has no
// notion of a "selected resolution" any more: hosts no longer hold one.
func cmdInspect(args []string) error {
	fileArg := ""
	raw := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--file":
			i++
			fileArg = mustValue(args, i, "--file")
		case arg == "--json":
			// JSON documents are always printed as JSON; the flag is accepted
			// for compatibility with older scripts.
		case arg == "--raw":
			raw = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			if fileArg != "" {
				return fmt.Errorf("unexpected argument %q", arg)
			}
			fileArg = arg
		}
	}
	if fileArg == "" {
		return fmt.Errorf("usage: relkit inspect --file <document>")
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
			fmt.Fprintln(os.Stderr, "# envelope signatures are not re-verified by inspect; the updater verifies them at install time")
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
  artifacts produce|merge   run the artifact producer / merge ArtifactSets
  bootstrap build          build the Runtime Bootstrap a host embeds
  payload build            build a payload table from a tree
  stage                    stage an immutable ArtifactSet
  inspect                  print a protobuf or JSON document
  release                  publish a staged release to an agent
  build                    build relkit components
  custody                  read the credential manifest by keyId
`)
}
