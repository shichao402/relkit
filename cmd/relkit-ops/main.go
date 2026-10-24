// Command relkit-ops runs the deployment operations a server administrator
// needs after the epoch: read the deployed state, plan and apply an upgrade,
// rotate a credential, and manage the server root key material.
//
// Everything that existed only to serve host-side resolution (certify, impact,
// escape, bundle, distribution, artifact-set, runtime-profile, catalog,
// operator, agent policy) is gone (requirements 14.5, 14.6, 14.7): the server
// signs its own Catalog and Profile when it deploys, so there is no manual
// signing entry point any more.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/shichao402/relkit/internal/jsonio"
	"github.com/shichao402/relkit/internal/vnext/diag"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/ops"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		writeError(err, contains(os.Args[1:], "--json"))
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: relkit-ops <status|plan|upgrade|rotate|key> [flags]")
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
	case "key":
		return cmdKey(args[1:])
	default:
		return fmt.Errorf("unknown relkit-ops command %q", args[0])
	}
}

func cmdKey(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("key requires generate, fingerprint or rename")
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
	case "rename":
		flags := flag.NewFlagSet("key rename", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		oldKeyID := flags.String("old-key-id", "", "existing key identifier (verified against the document)")
		newKeyID := flags.String("new-key-id", "", "new key identifier")
		oldPrivate := flags.String("old-private", "", "existing private key document")
		privatePath := flags.String("private", "", "new private key document")
		publicPath := flags.String("public", "", "new public key document")
		jsonOutput := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		result, err := ops.ReissueKeyPair(*oldKeyID, *newKeyID, *oldPrivate, *privatePath, *publicPath)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return printJSON(result)
		}
		fmt.Printf("renamed %s -> %s (%s)\npublic: %s\nprivate: %s\n",
			*oldKeyID, result.KeyID, result.Fingerprint, result.PublicPath, result.PrivatePath)
		return nil
	default:
		return fmt.Errorf("unknown key command %q", args[0])
	}
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
	fmt.Printf("operator: %s\nactive: %s (%s)\ncatalog sequence: %d\ncatalog current: %s\nupgrade available: %t\n",
		status.Operator,
		status.Profile.ServerRelease.Version,
		status.Profile.ServerRelease.Commit,
		status.CatalogSequence,
		status.CatalogCurrent,
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
	to := flags.String("to", "latest", "release version or latest")
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
		plan.Operator, plan.Target.Version, plan.Target.Commit, plan.OperationID)
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
	to := flags.String("to", "latest", "release version or latest")
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
			"schema":      "relkit.ops-upgrade-result/2",
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
