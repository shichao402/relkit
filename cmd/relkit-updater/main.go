package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/buildver"
	"github.com/shichao402/relkit/internal/updater"
	relkitproto "github.com/shichao402/relkit/proto"
)

// Overridden at release via -ldflags "-X main.version=...".
// Without ldflags (module channel: go install @vX) this resolves from
// debug.ReadBuildInfo; see internal/buildver.
var version = buildver.Placeholder

func main() {
	version = buildver.Resolve(version)
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// `relkit-updater proto` prints the relkit.updater.v2 schema this engine
	// speaks. Hosts feed it to buf generate; there is no other contract file.
	if len(args) == 1 && args[0] == "proto" {
		if _, err := os.Stdout.WriteString(relkitproto.UpdaterV2); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	fs := flag.NewFlagSet("relkit-updater", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	caps := fs.Bool("capabilities", false, "print the Capabilities event as one ProtoJSON line and exit")
	worker := fs.String("worker", "", "apply session id")
	dataDir := fs.String("data-dir", "", "engine data directory (required with --worker)")
	showVersion := fs.Bool("version", false, "print engine version")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unknown argument %q (subcommands: proto)\n", fs.Arg(0))
		return 2
	}
	eng := &updater.Engine{Version: version, Stdout: os.Stdout}
	if *showVersion {
		fmt.Fprintln(os.Stderr, version)
		return 0
	}
	if *caps {
		// capabilities-only: no profile was opened, so the handshake field
		// stays empty on purpose.
		if err := updater.WriteEvent(os.Stdout, &updaterv2.UpdaterEvent{
			Kind: &updaterv2.UpdaterEvent_Capabilities{Capabilities: eng.PublicCapabilities()},
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	if *worker != "" {
		if *dataDir == "" {
			fmt.Fprintln(os.Stderr, "--data-dir is required with --worker")
			return 2
		}
		if err := eng.RunWorker(*dataDir, *worker); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	req, err := updater.ReadRequest(os.Stdin)
	if err != nil {
		// A request that does not decode still gets a structured answer on
		// stdout; stderr keeps the diagnostic.
		fmt.Fprintln(os.Stderr, err)
		if werr := eng.RejectRequest(err); werr != nil {
			fmt.Fprintln(os.Stderr, werr)
			return 1
		}
		return 2
	}
	if err := eng.HandleRequest(ctx, req); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
