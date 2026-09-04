package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"cnb.cool/shichao402/relkit/internal/onboard"
)

func runAgentOnboard(out io.Writer, args []string) error {
	if len(args) == 0 || args[0] != "check" {
		return fmt.Errorf("usage: relkit-agent onboard check -config PATH -product ID [-json]")
	}
	fs := flag.NewFlagSet("onboard", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "relkit-agent.json", "agent config path")
	product := fs.String("product", "", "product id")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *product == "" {
		return fmt.Errorf("-product is required")
	}
	report, err := onboard.Evaluate(onboard.AgentChecklist(), onboard.EvalOptions{
		AgentConfig: *configPath,
		Product:     *product,
	})
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	for _, item := range report.Items {
		fmt.Fprintf(out, "%-28s %-16s %s\n", item.ID, item.Status, item.Evidence)
	}
	if onboard.AllGreen(report) {
		fmt.Fprintln(out, "onboard: complete")
	}
	return nil
}
