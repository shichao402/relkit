package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	relkitembed "cnb.cool/shichao402/relkit/embed"
	"cnb.cool/shichao402/relkit/internal/backends"
	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/directory"
	"cnb.cool/shichao402/relkit/internal/envelope"
	"cnb.cool/shichao402/relkit/internal/fallback"
	"cnb.cool/shichao402/relkit/internal/jsonio"
	"cnb.cool/shichao402/relkit/internal/keys"
	"cnb.cool/shichao402/relkit/internal/model"
	"cnb.cool/shichao402/relkit/internal/publish"
	"cnb.cool/shichao402/relkit/internal/publishproto"
	"cnb.cool/shichao402/relkit/internal/simulate"
	"cnb.cool/shichao402/relkit/internal/stage"
	"cnb.cool/shichao402/relkit/internal/verify"
	projver "cnb.cool/shichao402/relkit/version"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Overridden at release build time via -ldflags "-X main.version=...".
var version = "0.2.0"

func main() {
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

	configPath, showVersion, args, err := stripGlobalFlags(argv)
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
	case "init":
		err = cmdInit(rest)
	case "keygen":
		err = cmdKeygen(rest, configPath)
	case "version":
		err = cmdVersion(rest, configPath)
	case "stage":
		err = cmdStage(rest, configPath)
	case "inspect":
		err = cmdInspect(rest, configPath)
	case "simulate":
		err = cmdSimulate(rest, configPath)
	case "verify":
		err = cmdVerify(rest, configPath)
	case "publish":
		err = cmdPublish(rest, configPath)
	case "staged-put":
		err = cmdStagedPut(rest)
	case "fallback":
		err = cmdFallback(rest, configPath)
	case "directory":
		err = cmdDirectory(rest, configPath)
	case "agent-guide":
		err = cmdAgentGuide(rest)
	case "backends":
		err = cmdBackends(rest)
	case "onboard":
		err = cmdOnboard(rest, configPath)
	default:
		err = fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		return fail(err)
	}
	return 0
}

func cmdInit(args []string) error {
	var directory string
	product := ""
	force := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--product":
			i++
			product = mustValue(args, i, "--product")
		case arg == "--force":
			force = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		case directory == "":
			directory = arg
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}

	if directory == "" {
		directory = "."
	}
	target, err := filepath.Abs(filepath.Join(directory, config.ConfigName))
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil && !force {
		return fmt.Errorf("%s already exists; pass --force to overwrite", target)
	}
	if product == "" {
		product = filepath.Base(filepath.Dir(target))
	}
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return err
	}
	data, err := jsonio.MarshalPretty(config.Skeleton(product))
	if err != nil {
		return err
	}
	if err := jsonio.WritePath(target, data); err != nil {
		return err
	}
	fmt.Println("wrote " + target)

	versionPath := filepath.Join(filepath.Dir(target), projver.FileName)
	if _, err := os.Stat(versionPath); err == nil && !force {
		fmt.Println("kept existing " + versionPath)
	} else {
		doc, err := projver.Skeleton("0.1.0+1")
		if err != nil {
			return err
		}
		doc.Path = versionPath
		if err := doc.Write(); err != nil {
			return err
		}
		fmt.Println("wrote " + versionPath)
	}

	fmt.Println("next: 'relkit keygen --key-id k1 --out keys/', then paste the public key into signing.publicKeys")
	fmt.Println("version SSOT: edit with 'relkit version set|bump'; stage/publish read it when version args are omitted")
	return nil
}

func cmdKeygen(args []string, configPath string) error {
	keyID := ""
	outDir := "keys"
	force := false
	updateConfig := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--key-id":
			i++
			keyID = mustValue(args, i, "--key-id")
		case arg == "--out":
			i++
			outDir = mustValue(args, i, "--out")
		case arg == "--force":
			force = true
		case arg == "--update-config":
			updateConfig = true
		default:
			return fmt.Errorf("unknown flag %q", arg)
		}
	}
	if keyID == "" {
		return fmt.Errorf("--key-id is required")
	}
	if err := model.CheckIdentifier(keyID, "keyId"); err != nil {
		return err
	}

	resolvedOut, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(resolvedOut, 0o755); err != nil {
		return err
	}

	publicPath := filepath.Join(resolvedOut, keyID+".public.pb")
	privatePath := filepath.Join(resolvedOut, keyID+".private.pb")
	for _, path := range []string{publicPath, privatePath} {
		if _, err := os.Stat(path); err == nil && !force {
			return fmt.Errorf("%s already exists; pass --force to overwrite (this invalidates every client that embedded the old key)", path)
		}
	}

	seed, err := keys.GenerateSeed()
	if err != nil {
		return err
	}
	publicDoc := keys.PublicKeyDocument(keyID, seed)
	privateDoc := keys.PrivateKeyDocument(keyID, seed)

	publicBytes, err := rupv2.MarshalPublicKey(&publicDoc)
	if err != nil {
		return err
	}
	privateBytes, err := rupv2.MarshalPrivateKey(&privateDoc)
	if err != nil {
		return err
	}
	if err := jsonio.WritePath(publicPath, publicBytes); err != nil {
		return err
	}
	if err := jsonio.WritePath(privatePath, privateBytes); err != nil {
		return err
	}
	keys.RestrictPermissions(privatePath)

	fmt.Println("public  " + publicPath)
	fmt.Println("private " + privatePath)
	fmt.Println()
	fmt.Println("publicKeyBase64: " + base64.StdEncoding.EncodeToString(publicDoc.PublicKey))

	if updateConfig {
		target, adopted, err := recordPublicKey(configPath, keyID, publicDoc)
		if err != nil {
			return err
		}
		fmt.Println("recorded in " + target + " under signing.publicKeys")
		if adopted {
			fmt.Printf("signing.keyId set to %q (it named no configured key)\n", keyID)
		}
		fmt.Printf("To publish from this machine, set signing.privateKeyPath to %s (per-product key file; do not use a shared environment variable).\n", filepath.Base(privatePath))
	} else {
		fmt.Printf("Add that to signing.publicKeys in %s (or re-run with --update-config), and embed it in the client.\n", config.ConfigName)
	}
	fmt.Println()

	ignored, _ := keys.IsGitIgnored(privatePath, resolvedOut)
	if ignored == nil {
		fmt.Println("NOTE: could not determine whether the private key is git-ignored.")
	} else if !*ignored {
		fmt.Printf("WARNING: %s is NOT git-ignored.\n", filepath.Base(privatePath))
		fmt.Println("         Add '*.private.pb' to .gitignore before committing anything. A leaked signing key means anyone can forge updates for every client that trusts it.")
	}
	return nil
}

func cmdStage(args []string, configPath string) error {
	opts, err := parseStageArgs(args)
	if err != nil {
		return err
	}
	if err := resolveStageVersion(opts, configPath); err != nil {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	code, err := cfg.ResolveCode(opts.version, opts.code)
	if err != nil {
		return err
	}
	_, err = stage.Run(cfg, opts.version, code, opts.minFrom, opts.adds, opts.channel, opts.notes, opts.notesFile, opts.notesURL, opts.link, func(line string) {
		fmt.Println(line)
	})
	return err
}

func cmdInspect(args []string, configPath string) error {
	versionArg := ""
	fileArg := ""
	raw := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--file":
			i++
			fileArg = mustValue(args, i, "--file")
		case arg == "--raw":
			raw = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		case versionArg == "":
			versionArg = arg
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	var (
		document map[string]any
		message  proto.Message
	)
	if versionArg != "" {
		staged, err := stage.LoadStaged(cfg.Root, versionArg)
		if err != nil {
			return err
		}
		message = staged
	} else if fileArg != "" {
		if strings.EqualFold(filepath.Ext(fileArg), ".json") {
			if err := jsonio.LoadPathLenient(fileArg, &document); err != nil {
				return err
			}
		} else {
			message, err = loadProtoDocument(fileArg)
			if err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("pass either a version or --file")
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
			unsealed, err := unsealForInspection(env, cfg)
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

func cmdSimulate(args []string, configPath string) error {
	fromSpec := "all"
	indexPath := ""
	withStaged := ""
	channel := ""
	backendName := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--from":
			i++
			fromSpec = mustValue(args, i, "--from")
		case arg == "--index":
			i++
			indexPath = mustValue(args, i, "--index")
		case arg == "--with-staged":
			i++
			withStaged = mustValue(args, i, "--with-staged")
		case arg == "--channel":
			i++
			channel = mustValue(args, i, "--channel")
		case arg == "--backend":
			i++
			backendName = mustValue(args, i, "--backend")
		default:
			return fmt.Errorf("unknown flag %q", arg)
		}
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	_, err = simulate.Run(cfg, fromSpec, indexPath, channel, withStaged, backendName, func(line string) {
		fmt.Println(line)
	})
	return err
}

func cmdVerify(args []string, configPath string) error {
	channel := ""
	deep := false
	var to []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--channel":
			i++
			channel = mustValue(args, i, "--channel")
		case arg == "--to":
			i++
			to = append(to, mustValue(args, i, "--to"))
		case arg == "--deep":
			deep = true
		default:
			return fmt.Errorf("unknown flag %q", arg)
		}
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	findings, err := verify.Run(cfg, channel, to, deep, func(line string) {
		fmt.Println(line)
	})
	if err != nil {
		return err
	}
	if !findings.OK() {
		return exitCodeError{code: 1}
	}
	return nil
}

func cmdPublish(args []string, configPath string) error {
	versionArg := ""
	var to []string
	dryRun := false
	allowBackfill := false
	allowPartial := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--to":
			i++
			to = append(to, mustValue(args, i, "--to"))
		case arg == "--dry-run":
			dryRun = true
		case arg == "--allow-backfill":
			allowBackfill = true
		case arg == "--allow-partial":
			allowPartial = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		case versionArg == "":
			versionArg = arg
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	resolvedVersion, err := resolvePublishVersion(versionArg, configPath)
	if err != nil {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	_, err = publish.Run(cfg, resolvedVersion, to, dryRun, allowBackfill, allowPartial, func(line string) {
		fmt.Println(line)
	})
	return err
}

func cmdFallback(args []string, configPath string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: relkit fallback set --max-code N --url URL [options]")
	}
	subcommand := args[0]
	rest := args[1:]
	switch subcommand {
	case "set":
		return cmdFallbackSet(rest, configPath)
	default:
		return fmt.Errorf("unknown fallback subcommand %q (want set)", subcommand)
	}
}

func cmdFallbackSet(args []string, configPath string) error {
	var to []string
	dryRun := false
	clear := false
	minCode := int64(0)
	maxCode := int64(0)
	hasMaxCode := false
	manualURL := ""
	message := ""
	mandatory := false
	selectors := map[string]string{}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--to":
			i++
			to = append(to, mustValue(args, i, "--to"))
		case arg == "--dry-run":
			dryRun = true
		case arg == "--clear":
			clear = true
		case arg == "--min-code":
			i++
			value, err := strconv.ParseInt(mustValue(args, i, "--min-code"), 10, 64)
			if err != nil {
				return fmt.Errorf("--min-code: %w", err)
			}
			minCode = value
		case arg == "--max-code":
			i++
			value, err := strconv.ParseInt(mustValue(args, i, "--max-code"), 10, 64)
			if err != nil {
				return fmt.Errorf("--max-code: %w", err)
			}
			maxCode = value
			hasMaxCode = true
		case arg == "--url":
			i++
			manualURL = mustValue(args, i, "--url")
		case arg == "--message":
			i++
			message = mustValue(args, i, "--message")
		case arg == "--mandatory":
			mandatory = true
		case arg == "--selector":
			i++
			raw := mustValue(args, i, "--selector")
			key, value, ok := strings.Cut(raw, "=")
			if !ok || key == "" || value == "" {
				return fmt.Errorf("--selector expects key=value, got %q", raw)
			}
			selectors[key] = value
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}

	opts := fallback.Options{
		Clear:  clear,
		To:     to,
		DryRun: dryRun,
	}
	if !clear {
		if !hasMaxCode {
			return fmt.Errorf("--max-code is required (or pass --clear)")
		}
		opts.Rules = []fallback.RuleInput{{
			MinCode:   minCode,
			MaxCode:   maxCode,
			ManualURL: manualURL,
			Message:   message,
			Mandatory: mandatory,
			Selectors: selectors,
		}}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	_, err = fallback.Set(cfg, opts, func(line string) {
		fmt.Println(line)
	})
	return err
}

func cmdDirectory(args []string, configPath string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: relkit directory set [--from-config] [--service id=...,index-url=...] [--to name] [--dry-run]")
	}
	subcommand := args[0]
	rest := args[1:]
	switch subcommand {
	case "set":
		return cmdDirectorySet(rest, configPath)
	default:
		return fmt.Errorf("unknown directory subcommand %q (want set)", subcommand)
	}
}

func cmdDirectorySet(args []string, configPath string) error {
	var to []string
	dryRun := false
	fromConfig := false
	var services []directory.ServiceInput

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--to":
			i++
			to = append(to, mustValue(args, i, "--to"))
		case arg == "--dry-run":
			dryRun = true
		case arg == "--from-config":
			fromConfig = true
		case arg == "--service":
			i++
			service, err := parseDirectoryServiceFlag(mustValue(args, i, "--service"))
			if err != nil {
				return err
			}
			services = append(services, service)
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}

	if fromConfig && len(services) > 0 {
		return fmt.Errorf("--from-config cannot be combined with --service")
	}
	if !fromConfig && len(services) == 0 {
		// Default to config services when present; otherwise require explicit input.
		fromConfig = true
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	opts := directory.Options{
		To:     to,
		DryRun: dryRun,
	}
	if !fromConfig {
		opts.Services = services
	}
	_, err = directory.Set(cfg, opts, func(line string) {
		fmt.Println(line)
	})
	return err
}

func parseDirectoryServiceFlag(raw string) (directory.ServiceInput, error) {
	service := directory.ServiceInput{Priority: 100}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" || value == "" {
			return directory.ServiceInput{}, fmt.Errorf("--service expects key=value pairs, got %q in %q", part, raw)
		}
		switch strings.ToLower(key) {
		case "id":
			service.ID = value
		case "priority":
			n, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				return directory.ServiceInput{}, fmt.Errorf("--service priority: %w", err)
			}
			service.Priority = int32(n)
		case "index-url", "indexurl":
			service.IndexURL = value
		case "fallback-url", "fallbackurl":
			service.FallbackURL = value
		case "channel":
			service.Channel = value
		default:
			return directory.ServiceInput{}, fmt.Errorf("unknown --service field %q", key)
		}
	}
	if service.ID == "" || service.IndexURL == "" {
		return directory.ServiceInput{}, fmt.Errorf("--service requires id= and index-url=")
	}
	return service, nil
}

func cmdAgentGuide(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("agent-guide takes no arguments")
	}
	_, err := os.Stdout.WriteString(relkitembed.AgentGuide)
	return err
}

func cmdBackends(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("backends takes no arguments")
	}
	fmt.Println("backend types implemented in this build:")
	fmt.Println()
	types := backends.AvailableTypes()
	slices.Sort(types)
	for _, backendType := range types {
		summary, required, optional := backends.SummaryFor(backendType)
		fmt.Println("  " + backendType)
		fmt.Println("    " + summary)
		if len(required) == 0 {
			fmt.Println("    required: -")
		} else {
			fmt.Println("    required: " + strings.Join(required, ", "))
		}
		if len(optional) > 0 {
			fmt.Println("    optional: " + strings.Join(optional, ", "))
		}
		fmt.Println()
	}
	fmt.Println("Anything not listed here is unimplemented. See CLI.md section 6 for the full set")
	fmt.Println("and which of them still need to be built.")
	return nil
}

type stageArgs struct {
	version   string
	code      *int
	minFrom   int
	channel   string
	notes     string
	notesFile string
	notesURL  string
	link      bool
	adds      []stage.AddSpec
}

func parseStageArgs(args []string) (*stageArgs, error) {
	opts := &stageArgs{minFrom: 0}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--code":
			i++
			value, err := strconv.Atoi(mustValue(args, i, "--code"))
			if err != nil {
				return nil, err
			}
			opts.code = &value
		case arg == "--min-from":
			i++
			value, err := strconv.Atoi(mustValue(args, i, "--min-from"))
			if err != nil {
				return nil, err
			}
			opts.minFrom = value
		case arg == "--channel":
			i++
			opts.channel = mustValue(args, i, "--channel")
		case arg == "--notes":
			i++
			opts.notes = mustValue(args, i, "--notes")
		case arg == "--notes-file":
			i++
			opts.notesFile = mustValue(args, i, "--notes-file")
		case arg == "--notes-url":
			i++
			opts.notesURL = mustValue(args, i, "--notes-url")
		case arg == "--link":
			opts.link = true
		case arg == "--add":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--add requires a path")
			}
			pathValue := args[i+1]
			i++
			pairs := ""
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				pairs = args[i+1]
				i++
			}
			opts.adds = append(opts.adds, stage.AddSpec{Path: pathValue, PairsText: pairs})
		case strings.HasPrefix(arg, "-"):
			return nil, fmt.Errorf("unknown flag %q", arg)
		case opts.version == "":
			opts.version = arg
		default:
			return nil, fmt.Errorf("unexpected argument %q", arg)
		}
	}
	// Version may be omitted: cmdStage fills it from VERSION.json.
	return opts, nil
}

func recordPublicKey(configPath string, keyID string, publicDoc model.PublicKeyDocument) (string, bool, error) {
	target := configPath
	var err error
	if target == "" {
		target, err = config.FindConfig("")
		if err != nil {
			return "", false, err
		}
		if target == "" {
			return "", false, fmt.Errorf("--update-config needs a %s; run 'relkit init' first", config.ConfigName)
		}
	}

	var data map[string]any
	if err := jsonio.LoadPathLenient(target, &data); err != nil {
		return "", false, err
	}

	signing, _ := data["signing"].(map[string]any)
	if signing == nil {
		signing = map[string]any{}
		data["signing"] = signing
	}

	var entries []any
	if existing, ok := signing["publicKeys"].([]any); ok {
		for _, entry := range existing {
			obj, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if obj["keyId"] == keyID {
				continue
			}
			entries = append(entries, obj)
		}
	}
	entries = append(entries, map[string]any{
		"keyId":           keyID,
		"publicKeyBase64": base64.StdEncoding.EncodeToString(publicDoc.PublicKey),
	})
	signing["publicKeys"] = entries

	known := map[string]struct{}{keyID: {}}
	for _, entry := range entries {
		if obj, ok := entry.(map[string]any); ok {
			if key, _ := obj["keyId"].(string); key != "" {
				known[key] = struct{}{}
			}
		}
	}
	currentKeyID, _ := signing["keyId"].(string)
	adopted := false
	if _, ok := known[currentKeyID]; !ok {
		signing["keyId"] = keyID
		adopted = true
	}

	pretty, err := jsonio.MarshalPretty(data)
	if err != nil {
		return "", false, err
	}
	if err := jsonio.WritePath(target, pretty); err != nil {
		return "", false, err
	}
	return target, adopted, nil
}

func unsealForInspection(env *model.Envelope, cfg *config.Config) (proto.Message, error) {
	signedBy := []string{}
	for _, entry := range env.Signatures {
		if entry != nil && entry.KeyId != "" {
			signedBy = append(signedBy, entry.KeyId)
		}
	}
	slices.Sort(signedBy)

	trusted, err := cfg.TrustedPublicKeys()
	if err == nil && len(trusted) > 0 {
		index, openErr := envelope.OpenEnvelope(env, trusted)
		if openErr == nil {
			fmt.Fprintln(os.Stderr, "# signature ok, signed by "+strings.Join(signedBy, ", "))
			return index, nil
		}
		fmt.Fprintln(os.Stderr, "# WARNING: signature NOT verified: "+openErr.Error())
		fmt.Fprintln(os.Stderr, "# showing the payload anyway, unverified")
	} else {
		fmt.Fprintln(os.Stderr, "# signature not checked: no signing.publicKeys configured")
	}

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

func stripGlobalFlags(args []string) (string, bool, []string, error) {
	configPath := ""
	showVersion := false
	var remaining []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--version":
			// Only `relkit --version` is global. `relkit staged-put --version 1.2.3`
			// is the product version for that subcommand.
			if len(remaining) == 0 {
				showVersion = true
			} else {
				remaining = append(remaining, arg)
			}
		case arg == "--config":
			i++
			if i >= len(args) {
				return "", false, nil, fmt.Errorf("--config requires a value")
			}
			configPath = args[i]
		case strings.HasPrefix(arg, "--config="):
			configPath = strings.TrimPrefix(arg, "--config=")
		default:
			remaining = append(remaining, arg)
		}
	}
	return configPath, showVersion, remaining, nil
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
  init
  keygen
  version
  stage
  inspect
  simulate
  verify
  publish
  staged-put
  fallback
  directory
  agent-guide
  backends
  onboard
`)
}
