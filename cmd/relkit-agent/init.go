package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/shichao402/relkit/internal/agentadmin"
)

// runInit is the on-box bootstrap helper. It has exactly one job now: mint the
// one-shot credential that creates the first operator (requirement 13.2).
//
// Everything else that used to live here is gone on purpose. Product
// registration, token issuance, rotation and sharing are agent management
// operations, because the epoch upgrade put them behind the admin API so the
// panel can drive them (requirements 11.3, 13.6). `-product`, `-token-only` and
// `-share-with` are not deprecated spellings kept for a transition: they no
// longer exist, and a script that still calls them fails instead of half
// working.
func runInit(out io.Writer, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "relkit-agent.json", "agent config path")
	bootstrap := fs.Bool("bootstrap", false, "mint the one-shot credential that creates the first operator")
	reset := fs.Bool("reset", false, "with -bootstrap: replace a live bootstrap credential (the old value stops working)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*bootstrap {
		return fmt.Errorf("usage: relkit-agent init -config <path> -bootstrap\nproducts and tokens are managed through the admin surface, not this command")
	}
	return runInitBootstrap(out, *configPath, *reset)
}

// runInitBootstrap mints the one-shot credential. It refuses once an operator
// account exists: there is nothing left for the credential to authorise.
func runInitBootstrap(out io.Writer, configPath string, reset bool) error {
	raw, err := loadFileConfig(configPath)
	if err != nil {
		return err
	}
	store, err := openAdminStore(configPath, raw)
	if err != nil {
		return err
	}
	mint := store.BootstrapToken
	if reset {
		mint = store.ResetBootstrap
	}
	token, err := mint()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "admin state %s\n", store.Path())
	fmt.Fprintf(out, "\nCreate the first operator with this credential (shown once):\n")
	fmt.Fprintf(out, "  %s\n", token)
	fmt.Fprintf(out, "\nPOST it to /v1/admin/bootstrap with {\"bootstrap\":\"...\",\"username\":\"...\",\"password\":\"...\"},\n")
	fmt.Fprintf(out, "or paste it into the panel's first-run screen. It stops working the moment an account exists.\n")
	return nil
}

// bootstrapHint is the one message every path uses when the admin document has
// no accounts yet: the server cannot authorize anything, and the only way in is
// the on-box bootstrap.
func bootstrapHint(store *agentadmin.Store) string {
	if store != nil && store.BootstrapLive() {
		return "no operator account exists yet: finish the first-run bootstrap"
	}
	return "no operator account exists yet: run `relkit-agent init -bootstrap` on the server"
}
