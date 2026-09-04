package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/onboard"
)

func cmdOnboard(args []string, configPath string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: relkit onboard check|next|run|ack")
	}
	sub := args[0]
	rest := args[1:]
	hostRoot := ""
	asJSON := false
	checklist := "toolchain"
	filtered := make([]string, 0, len(rest))
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--json":
			asJSON = true
		case "--host":
			i++
			if i >= len(rest) {
				return fmt.Errorf("--host needs a path")
			}
			hostRoot = rest[i]
		case "--checklist":
			i++
			if i >= len(rest) {
				return fmt.Errorf("--checklist needs an id")
			}
			checklist = rest[i]
		default:
			filtered = append(filtered, rest[i])
		}
	}
	if hostRoot == "" {
		if configPath != "" {
			hostRoot = strings.TrimSuffix(configPath, config.ConfigName)
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			hostRoot = cwd
		}
	}

	cl, err := selectChecklist(checklist)
	if err != nil {
		return err
	}
	opts := onboard.EvalOptions{HostRoot: hostRoot}

	switch sub {
	case "check":
		report, err := onboard.Evaluate(cl, opts)
		if err != nil {
			return err
		}
		return printOnboard(report, asJSON)
	case "next":
		report, err := onboard.Evaluate(cl, opts)
		if err != nil {
			return err
		}
		next := onboard.NextFailed(report)
		if next == nil {
			fmt.Println("onboard: all required items passed")
			return nil
		}
		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(next)
		}
		fmt.Printf("%s\t%s\t%s\n", next.ID, next.Status, next.Remediation)
		if next.Evidence != "" {
			fmt.Println(next.Evidence)
		}
		return nil
	case "run":
		if len(filtered) < 1 {
			return fmt.Errorf("usage: relkit onboard run <step-id>")
		}
		id := filtered[0]
		allowed := map[string]bool{"repo.recovery-embed": true, "repo.client-contract": true}
		if !allowed[id] {
			return fmt.Errorf("step %q is not a runnable onboard action (allowed: repo.recovery-embed, repo.client-contract)", id)
		}
		cfgPath := configPath
		if cfgPath == "" {
			cfgPath = filepath.Join(hostRoot, config.ConfigName)
		}
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		switch id {
		case "repo.recovery-embed":
			if err := onboard.WriteRecoveryEmbed(hostRoot, cfg.Recovery); err != nil {
				return err
			}
			fmt.Println("wrote", onboard.RecoveryJSONPath(hostRoot))
		case "repo.client-contract":
			if err := onboard.WriteClientContract(hostRoot, cfg); err != nil {
				return err
			}
			fmt.Println("wrote", onboard.ContractPath(hostRoot))
		}
		return nil
	case "ack":
		if len(filtered) < 1 {
			return fmt.Errorf("usage: relkit onboard ack <step-id>")
		}
		id := filtered[0]
		if !onboard.IsManualItem(cl, id) {
			return fmt.Errorf("ack is only allowed for manual steps (got %q)", id)
		}
		note := ""
		if len(filtered) > 1 {
			note = strings.Join(filtered[1:], " ")
		}
		cfgPath := configPath
		if cfgPath == "" {
			cfgPath = filepath.Join(hostRoot, config.ConfigName)
		}
		cfg, _ := config.Load(cfgPath)
		if err := onboard.SaveAck(hostRoot, id, note, onboard.InputsHash(cfg)); err != nil {
			return err
		}
		fmt.Println("acked", id)
		return nil
	default:
		return fmt.Errorf("unknown onboard subcommand %q", sub)
	}
}

func selectChecklist(id string) (onboard.Checklist, error) {
	switch id {
	case "toolchain", "host":
		return onboard.HostChecklist(), nil
	case "agent-machine", "agent":
		return onboard.AgentChecklist(), nil
	default:
		return onboard.Checklist{}, fmt.Errorf("unknown checklist %q", id)
	}
}

func printOnboard(report *onboard.Report, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	for _, item := range report.Items {
		line := fmt.Sprintf("%-28s %-16s %s", item.ID, item.Status, item.Evidence)
		fmt.Println(line)
		if item.Remediation != "" && item.Status != onboard.StatusPassed {
			fmt.Println("  ->", item.Remediation)
		}
	}
	if onboard.AllGreen(report) {
		fmt.Println("onboard: complete")
	}
	return nil
}
