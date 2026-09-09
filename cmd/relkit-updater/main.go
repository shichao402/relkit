package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	updaterv1 "go.firoyang.com/relkit/api/updater/v1"
	"go.firoyang.com/relkit/internal/updater"
)

// Overridden at release via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("relkit-updater", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	caps := fs.Bool("capabilities", false, "print Capabilities event and exit")
	worker := fs.String("worker", "", "apply session id")
	dataDir := fs.String("data-dir", "", "engine data directory (required with --worker)")
	showVersion := fs.Bool("version", false, "print engine version")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	eng := &updater.Engine{Version: version, Stdout: os.Stdout}
	if *showVersion {
		fmt.Fprintln(os.Stderr, version)
		return 0
	}
	if *caps {
		req := &updaterv1.UpdaterRequest{
			Hello: &updaterv1.ClientHello{IpcMin: updater.IPCMin, IpcMax: updater.IPCMax},
		}
		// capabilities-only: emit caps without requiring profile
		if err := updater.WriteFrame(os.Stdout, &updaterv1.UpdaterEvent{
			Kind: &updaterv1.UpdaterEvent_Capabilities{Capabilities: capabilitiesEvent(version)},
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		_ = req
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
	req := &updaterv1.UpdaterRequest{}
	if err := updater.ReadFrame(os.Stdin, req); err != nil {
		if err == io.EOF {
			fmt.Fprintln(os.Stderr, "empty stdin")
			return 2
		}
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := eng.HandleRequest(ctx, req); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func capabilitiesEvent(ver string) *updaterv1.Capabilities {
	e := &updater.Engine{Version: ver}
	return e.PublicCapabilities()
}
