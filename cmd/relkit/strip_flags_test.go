package main

import "testing"

// stripGlobalFlags only lifts the leading `relkit --version`. Everything else
// stays in argv so each command rejects its own unknown flags; `--version` and
// `--project` after a subcommand are command-scoped values, not globals.
func TestStripGlobalVersionOnlyBeforeCommand(t *testing.T) {
	show, rest, err := stripGlobalFlags([]string{"--version"})
	if err != nil || !show || len(rest) != 0 {
		t.Fatalf("relkit --version: show=%v rest=%v err=%v", show, rest, err)
	}

	show, rest, err = stripGlobalFlags([]string{"release", "staged.tar.gz", "--version", "0.0.999", "--product", "dec"})
	if err != nil {
		t.Fatal(err)
	}
	if show {
		t.Fatal("subcommand --version must not print the CLI version")
	}
	if len(rest) != 6 || rest[0] != "release" || rest[2] != "--version" || rest[3] != "0.0.999" {
		t.Fatalf("rest=%v", rest)
	}
}

func TestStripGlobalVersionAfterCommandStaysInArgv(t *testing.T) {
	show, rest, err := stripGlobalFlags([]string{"verify", "released", "--version", "v0.6.23"})
	if err != nil {
		t.Fatal(err)
	}
	if show {
		t.Fatal("verify released --version must not print the CLI version")
	}
	if len(rest) != 4 || rest[0] != "verify" {
		t.Fatalf("rest=%v", rest)
	}
}

func TestStripGlobalFlagsPassesOtherFlagsThrough(t *testing.T) {
	show, rest, err := stripGlobalFlags([]string{"stage", "--project", "relkit.project.json", "--code", "3"})
	if err != nil {
		t.Fatal(err)
	}
	if show {
		t.Fatal("stage must not print the CLI version")
	}
	if len(rest) != 5 || rest[0] != "stage" || rest[1] != "--project" {
		t.Fatalf("rest=%v", rest)
	}
}
