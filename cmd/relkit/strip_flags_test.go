package main

import "testing"

func TestStripGlobalVersionOnlyBeforeCommand(t *testing.T) {
	_, show, rest, err := stripGlobalFlags([]string{"--version"})
	if err != nil || !show || len(rest) != 0 {
		t.Fatalf("relkit --version: show=%v rest=%v err=%v", show, rest, err)
	}

	_, show, rest, err = stripGlobalFlags([]string{"staged-put", "staged.tar.gz", "--version", "0.0.999", "--product", "dec"})
	if err != nil {
		t.Fatal(err)
	}
	if show {
		t.Fatal("subcommand --version must not print the CLI version")
	}
	if len(rest) != 6 || rest[0] != "staged-put" || rest[2] != "--version" || rest[3] != "0.0.999" {
		t.Fatalf("rest=%v", rest)
	}
}
