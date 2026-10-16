package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/shichao402/relkit/internal/jsonio"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/diag"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/offlinebundle"
	"github.com/shichao402/relkit/internal/vnext/ops"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/resolution"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		writeError(err, contains(os.Args[1:], "--json"))
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: relkit-ops <status|plan|upgrade|rotate|impact|certify|bootstrap|bundle|distribution|catalog|operator|runtime-profile|artifact-set|agent|key> [flags]")
	}
	switch args[0] {
	case "status":
		return cmdStatus(args[1:])
	case "plan":
		return cmdPlan(args[1:])
	case "upgrade":
		return cmdUpgrade(ctx, args[1:])
	case "rotate":
		return cmdRotate(ctx, args[1:])
	case "certify":
		return cmdCertify(ctx, args[1:])
	case "impact":
		return cmdImpact(args[1:])
	case "bootstrap":
		return cmdBootstrap(args[1:])
	case "bundle":
		return cmdBundle(args[1:])
	case "distribution":
		return cmdDistribution(args[1:])
	case "catalog":
		return cmdCatalog(args[1:])
	case "operator":
		return cmdOperator(args[1:])
	case "runtime-profile":
		return cmdRuntimeProfile(ctx, args[1:])
	case "artifact-set":
		return cmdArtifactSet(ctx, args[1:])
	case "agent":
		return cmdAgent(args[1:])
	case "key":
		return cmdKey(args[1:])
	default:
		return fmt.Errorf("unknown relkit-ops command %q", args[0])
	}
}

func cmdAgent(args []string) error {
	if len(args) == 0 || args[0] != "policy" {
		return fmt.Errorf("agent requires policy")
	}
	flags := flag.NewFlagSet("agent policy", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configPath := flags.String("config", "", "existing relkit-agent JSON config")
	policyPath := flags.String("policy", "", "relkit.agent-product-policy/1 document")
	backupPath := flags.String("backup", "", "new rollback backup path")
	execute := flags.Bool("execute", false, "atomically apply the reviewed policy")
	jsonOutput := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	result, err := ops.ApplyAgentPolicy(ops.AgentPolicyOptions{
		Config: *configPath, Policy: *policyPath, Backup: *backupPath, Execute: *execute,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(result)
	}
	if !*execute {
		fmt.Printf("plan only: changed=%t products=%v; pass --execute with --backup to apply\n",
			result.Changed, result.ChangedProducts)
		return nil
	}
	fmt.Printf("agent policy applied: changed=%t products=%v backup=%s\n",
		result.Changed, result.ChangedProducts, result.Backup)
	return nil
}

func cmdDistribution(args []string) error {
	if len(args) == 0 || args[0] != "create" {
		return fmt.Errorf("distribution requires create")
	}
	flags := flag.NewFlagSet("distribution create", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	version := flags.String("version", "", "immutable release version, for example v0.6.1")
	manifest := flags.String("manifest", "", "relkit.release/1 manifest")
	assets := flags.String("assets", "", "directory containing downloaded release assets")
	sourceBase := flags.String("source-base", "", "immutable release asset URL prefix")
	output := flags.String("out", "", "new distribution row JSON")
	jsonOutput := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	value, err := ops.BuildDistribution(ops.BuildDistributionOptions{
		Version: *version, ManifestPath: *manifest, Assets: *assets,
		SourceBase: *sourceBase, Output: *output,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(value)
	}
	fmt.Printf("created distribution %s (%s) -> %s\n", value.Version, value.Digest, *output)
	return nil
}

func cmdArtifactSet(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("artifact-set requires push or pull")
	}
	switch args[0] {
	case "push":
		flags := flag.NewFlagSet("artifact-set push", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		projectPath := flags.String("project", "relkit.project.json", "project declaration")
		artifactSetPath := flags.String("artifact-set", "", "ArtifactSet JSON")
		rawURL := flags.String("url", "", "immutable CI transfer URL")
		usernameEnv := flags.String("username-env", "MIRRORS_USERNAME", "environment variable containing upload username")
		passwordEnv := flags.String("password-env", "MIRRORS_PASSWORD", "environment variable containing upload password")
		jsonOutput := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		result, err := ops.PushArtifactSet(ctx, ops.PushArtifactSetOptions{
			ProjectPath: *projectPath, ArtifactSetPath: *artifactSetPath, URL: *rawURL,
			Username: os.Getenv(*usernameEnv), Password: os.Getenv(*passwordEnv),
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return printJSON(result)
		}
		fmt.Printf("uploaded %s %s (%s)\n", result.Product, result.Version, result.ArchiveSHA256)
		return nil
	case "pull":
		flags := flag.NewFlagSet("artifact-set pull", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		projectPath := flags.String("project", "relkit.project.json", "project declaration")
		rawURL := flags.String("url", "", "immutable CI transfer URL")
		digest := flags.String("sha256", "", "expected sha256:... archive digest")
		output := flags.String("out-dir", "", "new ArtifactSet output directory")
		usernameEnv := flags.String("username-env", "MIRRORS_USERNAME", "optional environment variable containing read username")
		passwordEnv := flags.String("password-env", "MIRRORS_PASSWORD", "optional environment variable containing read password")
		jsonOutput := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		result, err := ops.PullArtifactSet(ctx, ops.PullArtifactSetOptions{
			ProjectPath: *projectPath, URL: *rawURL, ExpectedSHA256: *digest,
			OutputDirectory: *output, Username: os.Getenv(*usernameEnv), Password: os.Getenv(*passwordEnv),
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return printJSON(result)
		}
		fmt.Printf("downloaded %s %s (%s) -> %s\n", result.Product, result.Version, result.ArchiveSHA256, result.OutputDirectory)
		return nil
	default:
		return fmt.Errorf("unknown artifact-set command %q", args[0])
	}
}

func cmdKey(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("key requires generate or fingerprint")
	}
	switch args[0] {
	case "generate":
		flags := flag.NewFlagSet("key generate", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		keyID := flags.String("key-id", "", "key identifier")
		privatePath := flags.String("private", "", "new private key document")
		publicPath := flags.String("public", "", "new public key document")
		jsonOutput := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		result, err := ops.GenerateKeyPair(*keyID, *privatePath, *publicPath)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return printJSON(result)
		}
		fmt.Printf("generated %s (%s)\npublic: %s\nprivate: %s\n",
			result.KeyID, result.Fingerprint, result.PublicPath, result.PrivatePath)
		return nil
	case "fingerprint":
		flags := flag.NewFlagSet("key fingerprint", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		publicPath := flags.String("public", "", "public key document")
		jsonOutput := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		result, err := ops.FingerprintKey(*publicPath)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return printJSON(result)
		}
		fmt.Printf("%s %s\n", result.KeyID, result.Fingerprint)
		return nil
	default:
		return fmt.Errorf("unknown key command %q", args[0])
	}
}

func cmdRuntimeProfile(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("runtime-profile requires create or publish")
	}
	if args[0] == "publish" {
		flags := flag.NewFlagSet("runtime-profile publish", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		projectPath := flags.String("project", "relkit.project.json", "project declaration")
		agentURL := flags.String("agent", os.Getenv("RELKIT_AGENT_URL"), "product agent URL")
		tokenFile := flags.String("token-file", "", "file containing the product upload token")
		output := flags.String("out", "", "signed Runtime Profile JSON")
		replace := flags.Bool("replace", false, "atomically replace an existing output")
		dryRun := flags.Bool("dry-run", false, "sign and validate without publishing")
		jsonOutput := flags.Bool("json", false, "print JSON")
		var targets stringList
		flags.Var(&targets, "to", "publication backend (repeatable)")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		token := strings.TrimSpace(os.Getenv("RELKIT_UPLOAD_TOKEN"))
		if *tokenFile != "" {
			raw, err := os.ReadFile(*tokenFile)
			if err != nil {
				return err
			}
			token = strings.TrimSpace(string(raw))
		}
		result, err := ops.PublishRuntimeProfile(ctx, ops.PublishRuntimeProfileOptions{
			ProjectPath: *projectPath, AgentURL: *agentURL, Token: token, To: targets,
			DryRun: *dryRun, Output: *output, Replace: *replace,
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return printJSON(result)
		}
		for _, line := range result.Log {
			fmt.Println(line)
		}
		if *dryRun {
			fmt.Printf("validated Runtime Profile %s sequence %d (dry run)\n", result.Profile.Digest, result.Profile.ProfileVersion)
		} else {
			fmt.Printf("published Runtime Profile %s sequence %d -> %s\n", result.Profile.Digest, result.Profile.ProfileVersion, *output)
		}
		return nil
	}
	if args[0] != "create" {
		return fmt.Errorf("unknown runtime-profile command %q", args[0])
	}
	flags := flag.NewFlagSet("runtime-profile create", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectPath := flags.String("project", "relkit.project.json", "project declaration")
	version := flags.Int64("profile-version", 1, "monotonic Runtime Profile version")
	output := flags.String("out", "", "signed Runtime Profile JSON")
	replace := flags.Bool("replace", false, "atomically replace an existing output")
	jsonOutput := flags.Bool("json", false, "print JSON")
	var entryURLs stringList
	var keys stringList
	flags.Var(&entryURLs, "entry-url", "consumer-visible RUP directory URL (repeatable)")
	flags.Var(&keys, "key", "product private key document (repeatable for rotation)")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	value, err := ops.CreateRuntimeProfile(ops.CreateRuntimeProfileOptions{
		ProjectPath: *projectPath, EntryURLs: entryURLs, ProfileVersion: *version,
		KeyPaths: keys, Output: *output, Replace: *replace,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(value)
	}
	fmt.Printf("created Runtime Profile %s sequence %d -> %s\n", value.Digest, value.ProfileVersion, *output)
	return nil
}

func cmdImpact(args []string) error {
	if len(args) == 0 || args[0] != "generate" {
		return fmt.Errorf("impact requires generate")
	}
	flags := flag.NewFlagSet("impact generate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	productionCatalog := flags.String("production-catalog", "", "signed production catalog")
	candidateCatalog := flags.String("candidate-catalog", "", "signed candidate catalog")
	candidateProfile := flags.String("candidate-profile", "", "signed candidate Operator Profile")
	candidateDigest := flags.String("candidate", "", "candidate distribution digest")
	candidateRoot := flags.String("candidate-root-fingerprint", "", "Candidate Root fingerprint")
	risk := flags.String("risk", "", "documentation, single-capability, contract, or cross-cutting")
	output := flags.String("out", "", "immutable impact manifest")
	var productionTrust stringList
	var candidateTrust stringList
	flags.Var(&productionTrust, "production-trust-key", "Distribution Root public key (repeatable)")
	flags.Var(&candidateTrust, "candidate-trust-key", "Candidate Root public key (repeatable)")
	jsonOutput := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	impact, err := ops.GenerateImpact(ops.GenerateImpactOptions{
		ProductionCatalogPath: *productionCatalog, CandidateCatalogPath: *candidateCatalog,
		ProductionTrustKeyPaths: productionTrust, CandidateTrustKeyPaths: candidateTrust,
		CandidateDigest: *candidateDigest, CandidateProfilePath: *candidateProfile,
		CandidateRootFingerprint: *candidateRoot, Risk: ops.Risk(*risk),
	})
	if err != nil {
		return err
	}
	if *output != "" {
		if err := document.WriteNew(*output, impact); err != nil {
			return err
		}
	}
	if *jsonOutput {
		return printJSON(impact)
	}
	fmt.Printf("candidate %s impacts %d components and %d contracts (%s)\n",
		impact.CandidateDigest, len(impact.AffectedComponents), len(impact.AffectedCapabilities), impact.Risk)
	return nil
}

func cmdCertify(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("certify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	fleetPath := flags.String("fleet", "", "fleet registration")
	impactPath := flags.String("impact", "", "candidate impact manifest")
	output := flags.String("out", "", "immutable certification report")
	full := flags.Bool("full", false, "select every active registered host")
	jsonOutput := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *fleetPath == "" || *impactPath == "" {
		return fmt.Errorf("--fleet and --impact are required")
	}
	fleet, err := ops.LoadFleet(*fleetPath)
	if err != nil {
		return err
	}
	impact, err := ops.LoadImpact(*impactPath)
	if err != nil {
		return err
	}
	report, err := ops.RunCertification(ctx, *fleet, *impact, *full)
	if err != nil {
		return err
	}
	if *output != "" {
		if err := document.WriteNew(*output, report); err != nil {
			return err
		}
	}
	if *jsonOutput {
		return printJSON(report)
	}
	fmt.Printf("candidate %s: passed=%t, hosts=%d, checks=%d\n",
		report.CandidateDigest, report.Passed, len(report.SelectedHosts), len(report.Results))
	if !report.Passed {
		return fmt.Errorf("candidate certification did not pass")
	}
	return nil
}

func cmdBundle(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("bundle requires create, verify, or install")
	}
	switch args[0] {
	case "create":
		flags := flag.NewFlagSet("bundle create", flag.ContinueOnError)
		bundlePath := flags.String("resolution-bundle", "", "signed resolution bundle")
		projectPath := flags.String("project", "relkit.project.json", "external project declaration")
		output := flags.String("out", "", "new distribution bundle directory")
		candidateRoot := flags.String("candidate-root-fingerprint", "", "authorized Candidate Root fingerprint")
		var assets keyValueList
		flags.Var(&assets, "asset", "digest=path (repeat for every distribution file)")
		jsonOutput := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		product, bundle, policy, err := bundleInputs(*projectPath, *bundlePath, *candidateRoot)
		if err != nil {
			return err
		}
		_ = product
		manifest, err := offlinebundle.Create(offlinebundle.CreateOptions{
			ResolutionBundle: *bundle,
			Policy:           policy,
			Assets:           assets,
			OutputDirectory:  *output,
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return printJSON(manifest)
		}
		fmt.Printf("created complete distribution bundle %s for %s\n", *output, manifest.ResolutionDigest)
		return nil
	case "verify":
		flags := flag.NewFlagSet("bundle verify", flag.ContinueOnError)
		directory := flags.String("bundle", "", "distribution bundle directory")
		projectPath := flags.String("project", "relkit.project.json", "external project declaration")
		candidateRoot := flags.String("candidate-root-fingerprint", "", "authorized Candidate Root fingerprint")
		jsonOutput := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		product, err := project.Load(*projectPath)
		if err != nil {
			return err
		}
		manifest, bundle, err := offlinebundle.Verify(*directory, resolution.BundlePolicy{
			DistributionRootFingerprint: product.TrustFingerprint,
			CandidateRootFingerprint:    *candidateRoot,
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return printJSON(map[string]any{
				"schema":   "relkit.ops-bundle-verify-result/1",
				"ok":       true,
				"manifest": manifest,
				"identity": bundle.Resolution.Identity,
			})
		}
		fmt.Printf("verified bundle %s (%s)\n", manifest.ResolutionDigest, bundle.Resolution.Identity)
		return nil
	case "install":
		flags := flag.NewFlagSet("bundle install", flag.ContinueOnError)
		directory := flags.String("bundle", "", "distribution bundle directory")
		projectPath := flags.String("project", "relkit.project.json", "project declaration")
		projectRoot := flags.String("project-root", "", "project checkout root")
		home := flags.String("home", "", "Relkit user configuration root")
		target := flags.String("target", "", "target platform")
		jsonOutput := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		resolved, err := offlinebundle.Install(offlinebundle.InstallOptions{
			Directory: *directory, ProjectPath: *projectPath, ProjectRoot: *projectRoot,
			RelkitHome: *home, Target: *target,
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return printJSON(map[string]any{
				"schema":     "relkit.ops-bundle-install-result/1",
				"ok":         true,
				"resolution": resolved,
			})
		}
		fmt.Printf("installed offline resolution %s\n", resolved.Digest)
		return nil
	default:
		return fmt.Errorf("unknown bundle command %q", args[0])
	}
}

func bundleInputs(projectPath, bundlePath, candidateRoot string) (*project.Document, *resolution.Bundle, resolution.BundlePolicy, error) {
	product, err := project.Load(projectPath)
	if err != nil {
		return nil, nil, resolution.BundlePolicy{}, err
	}
	bundle, err := resolution.LoadBundle(bundlePath)
	if err != nil {
		return nil, nil, resolution.BundlePolicy{}, err
	}
	policy := resolution.BundlePolicy{
		DistributionRootFingerprint: product.TrustFingerprint,
		CandidateRootFingerprint:    candidateRoot,
		OperatorDeploymentID:        bundle.Operator.DeploymentID,
		Identity:                    bundle.Resolution.Identity,
	}
	return product, bundle, policy, nil
}

func cmdBootstrap(args []string) error {
	if len(args) == 0 || args[0] != "provision" {
		return fmt.Errorf("bootstrap requires provision")
	}
	flags := flag.NewFlagSet("bootstrap provision", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	inventoryPath := flags.String("inventory", "", "Operator inventory")
	home := flags.String("home", "", "Relkit user configuration root")
	jsonOutput := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	inventory, err := loadInventory(*inventoryPath)
	if err != nil {
		return err
	}
	path, err := ops.ProvisionBootstrap(*inventory, *home)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(map[string]any{
			"schema": "relkit.ops-bootstrap-result/1",
			"ok":     true,
			"path":   path,
		})
	}
	fmt.Println("provisioned " + path)
	return nil
}

func cmdStatus(args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	inventoryPath := flags.String("inventory", "", "Operator inventory")
	jsonOutput := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	inventory, err := loadInventory(*inventoryPath)
	if err != nil {
		return err
	}
	status, err := ops.ReadStatus(*inventory)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(status)
	}
	fmt.Printf("operator: %s\nactive: %s (%s)\ncatalog sequence: %d\nupgrade available: %t\n",
		status.Operator,
		status.Profile.ServerRelease.Version,
		status.Profile.ServerRelease.DistributionDigest,
		status.CatalogSequence,
		status.UpgradeAvailable,
	)
	if len(status.IncompleteOperations) > 0 {
		fmt.Printf("incomplete operations: %d\n", len(status.IncompleteOperations))
	}
	return nil
}

func cmdPlan(args []string) error {
	flags := flag.NewFlagSet("plan", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	inventoryPath := flags.String("inventory", "", "Operator inventory")
	to := flags.String("to", "latest", "certified version or latest")
	output := flags.String("out", "", "write immutable plan")
	jsonOutput := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	inventory, err := loadInventory(*inventoryPath)
	if err != nil {
		return err
	}
	plan, err := ops.PlanUpgrade(*inventory, *to)
	if err != nil {
		return err
	}
	if *output != "" {
		if err := ops.WriteUpgradePlan(*output, *plan); err != nil {
			return err
		}
	}
	if *jsonOutput {
		return printJSON(plan)
	}
	fmt.Printf("operator: %s\ntarget: %s (%s)\noperation: %s\n",
		plan.Operator, plan.Target.Version, plan.Target.Digest, plan.OperationID)
	return nil
}

func cmdUpgrade(ctx context.Context, args []string) error {
	operatorID := ""
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		operatorID = args[0]
		args = args[1:]
	}
	flags := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	inventoryPath := flags.String("inventory", "", "Operator inventory")
	to := flags.String("to", "latest", "certified version or latest")
	planPath := flags.String("plan", "", "previously written immutable plan")
	execute := flags.Bool("execute", false, "execute the planned transaction")
	jsonOutput := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	inventory, err := loadInventory(*inventoryPath)
	if err != nil {
		return err
	}
	if operatorID != "" && operatorID != inventory.Operator {
		return fmt.Errorf("requested operator %q does not match inventory %q", operatorID, inventory.Operator)
	}
	var plan ops.UpgradePlan
	if *planPath != "" {
		if err := document.Load(*planPath, &plan); err != nil {
			return err
		}
	} else {
		value, err := ops.PlanUpgrade(*inventory, *to)
		if err != nil {
			return err
		}
		plan = *value
	}
	if !*execute {
		if *jsonOutput {
			return printJSON(plan)
		}
		fmt.Printf("plan only: %s -> %s (%s); pass --execute to apply\n", plan.Operator, plan.Target.Version, plan.OperationID)
		return nil
	}
	profile, err := ops.ApplyUpgrade(ctx, *inventory, plan)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(map[string]any{
			"schema":      "relkit.ops-upgrade-result/1",
			"ok":          true,
			"operationId": plan.OperationID,
			"profile":     profile,
		})
	}
	fmt.Printf("upgraded %s to %s\n", profile.DeploymentID, profile.ServerRelease.Version)
	return nil
}

func cmdRotate(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "" || args[0][0] == '-' {
		return fmt.Errorf("rotate requires a credential name")
	}
	credential := args[0]
	flags := flag.NewFlagSet("rotate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	inventoryPath := flags.String("inventory", "", "Operator inventory")
	execute := flags.Bool("execute", false, "execute the rotation")
	jsonOutput := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if !*execute {
		return fmt.Errorf("rotate is mutating; pass --execute after reviewing relkit-ops status")
	}
	inventory, err := loadInventory(*inventoryPath)
	if err != nil {
		return err
	}
	profile, err := ops.RotateCredential(ctx, *inventory, credential)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(map[string]any{
			"schema":     "relkit.ops-rotate-result/1",
			"ok":         true,
			"credential": credential,
			"profile":    profile,
		})
	}
	fmt.Printf("rotated %s for %s; profile sequence %d is active\n", credential, profile.DeploymentID, profile.Sequence)
	return nil
}

func cmdCatalog(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("catalog requires put, sign, or transition")
	}
	switch args[0] {
	case "put":
		flags := flag.NewFlagSet("catalog put", flag.ContinueOnError)
		current := flags.String("catalog", "", "existing signed catalog (omit for initial catalog)")
		distribution := flags.String("distribution", "", "distribution row JSON")
		output := flags.String("out", "", "updated signed catalog JSON")
		key := flags.String("key", "", "private key document")
		authority := flags.String("authority", string(ops.AuthorityProduction), "production or candidate")
		var trust stringList
		flags.Var(&trust, "trust-key", "trusted public key document (repeatable)")
		jsonOutput := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := ops.PutDistribution(ops.PutDistributionOptions{
			CatalogPath: *current, Distribution: *distribution, Output: *output,
			KeyPath: *key, TrustKeyPaths: trust, Authority: ops.CatalogAuthority(*authority),
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return printJSON(value)
		}
		fmt.Printf("catalog sequence %d includes distribution %s\n", value.Sequence, *distribution)
		return nil
	case "sign":
		flags := flag.NewFlagSet("catalog sign", flag.ContinueOnError)
		input := flags.String("input", "", "catalog JSON")
		output := flags.String("out", "", "signed catalog JSON")
		key := flags.String("key", "", "private key document")
		authority := flags.String("authority", string(ops.AuthorityProduction), "production or candidate")
		add := flags.Bool("add-signature", false, "preserve signatures and add one")
		jsonOutput := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := ops.SignCatalog(ops.SignCatalogOptions{
			Input: *input, Output: *output, KeyPath: *key, Add: *add, Authority: ops.CatalogAuthority(*authority),
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return printJSON(value)
		}
		fmt.Printf("signed catalog sequence %d -> %s\n", value.Sequence, *output)
		return nil
	case "transition":
		flags := flag.NewFlagSet("catalog transition", flag.ContinueOnError)
		input := flags.String("input", "", "signed catalog JSON")
		output := flags.String("out", "", "updated catalog JSON")
		key := flags.String("key", "", "private key document")
		digest := flags.String("distribution", "", "immutable distribution digest")
		state := flags.String("state", "", "candidate, certified, superseded, or revoked")
		certificationReport := flags.String("certification-report", "", "passing relkit.certification-report/1")
		authority := flags.String("authority", string(ops.AuthorityProduction), "production or candidate")
		var trust stringList
		flags.Var(&trust, "trust-key", "trusted public key document (repeatable)")
		jsonOutput := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		value, err := ops.TransitionCatalog(ops.TransitionCatalogOptions{
			Input: *input, Output: *output, Distribution: *digest, State: catalog.State(*state),
			KeyPath: *key, TrustKeyPaths: trust, Authority: ops.CatalogAuthority(*authority),
			CertificationReportPath: *certificationReport,
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return printJSON(value)
		}
		fmt.Printf("catalog sequence %d: %s -> %s\n", value.Sequence, *digest, *state)
		return nil
	default:
		return fmt.Errorf("unknown catalog command %q", args[0])
	}
}

func cmdOperator(args []string) error {
	if len(args) == 0 || args[0] != "sign" {
		return fmt.Errorf("operator requires sign")
	}
	flags := flag.NewFlagSet("operator sign", flag.ContinueOnError)
	input := flags.String("input", "", "Operator Profile JSON")
	output := flags.String("out", "", "signed Operator Profile JSON")
	key := flags.String("key", "", "private key document")
	add := flags.Bool("add-signature", false, "preserve signatures and add one")
	jsonOutput := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	value, err := ops.SignOperator(ops.SignOperatorOptions{
		Input: *input, Output: *output, KeyPath: *key, Add: *add,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(value)
	}
	fmt.Printf("signed Operator Profile %s sequence %d -> %s\n", value.DeploymentID, value.Sequence, *output)
	return nil
}

func loadInventory(path string) (*ops.Inventory, error) {
	if path == "" {
		path = os.Getenv("RELKIT_OPS_INVENTORY")
	}
	if path == "" {
		return nil, fmt.Errorf("--inventory or RELKIT_OPS_INVENTORY is required")
	}
	return ops.LoadInventory(path)
}

func printJSON(value any) error {
	data, err := jsonio.MarshalPretty(value)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}

func writeError(err error, jsonOutput bool) {
	if !jsonOutput {
		fmt.Fprintln(os.Stderr, "relkit-ops:", err)
		return
	}
	envelope := diag.Envelope{
		Code:     "ops.command_failed",
		Severity: diag.SeverityError,
		Stage:    "command",
		Message:  err.Error(),
	}
	var diagnostic diag.Error
	if errors.As(err, &diagnostic) {
		envelope = diagnostic.Diagnostic
		if diagnostic.Cause != nil {
			if envelope.Evidence == nil {
				envelope.Evidence = map[string]any{}
			}
			envelope.Evidence["cause"] = diagnostic.Cause.Error()
		}
	}
	_ = json.NewEncoder(os.Stderr).Encode(envelope)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type stringList []string

func (s *stringList) String() string { return fmt.Sprint([]string(*s)) }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type keyValueList map[string]string

func (k *keyValueList) String() string { return fmt.Sprint(map[string]string(*k)) }
func (k *keyValueList) Set(value string) error {
	key, path, ok := strings.Cut(value, "=")
	if !ok || key == "" || path == "" {
		return fmt.Errorf("expected digest=path")
	}
	if *k == nil {
		*k = map[string]string{}
	}
	if _, exists := (*k)[key]; exists {
		return fmt.Errorf("duplicate asset %s", key)
	}
	(*k)[key] = path
	return nil
}
