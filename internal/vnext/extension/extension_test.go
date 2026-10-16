package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestRunUsesJSONBoundaryAndEnvironmentAllowlist(t *testing.T) {
	t.Setenv("RELKIT_EXTENSION_HELPER", "1")
	t.Setenv("RELKIT_EXTENSION_ALLOWED", "visible")
	t.Setenv("RELKIT_EXTENSION_SECRET", "must-not-leak")
	spec := Spec{
		Schema:         Schema,
		Kind:           KindVersionProvider,
		Command:        []string{os.Args[0], "-test.run=TestExtensionHelperProcess"},
		StdinSchema:    "example.input/1",
		StdoutSchema:   "example.output/1",
		TimeoutSeconds: 10,
		EnvAllowlist:   []string{"RELKIT_EXTENSION_HELPER", "RELKIT_EXTENSION_ALLOWED"},
	}
	var output struct {
		Schema        string `json:"schema"`
		Allowed       string `json:"allowed"`
		SecretVisible bool   `json:"secretVisible"`
	}
	if err := Run(context.Background(), spec, map[string]any{"schema": "example.input/1"}, &output); err != nil {
		t.Fatal(err)
	}
	if output.Schema != "example.output/1" || output.Allowed != "visible" || output.SecretVisible {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestExtensionHelperProcess(t *testing.T) {
	if os.Getenv("RELKIT_EXTENSION_HELPER") != "1" {
		return
	}
	_, _ = io.ReadAll(os.Stdin)
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"schema":        "example.output/1",
		"allowed":       os.Getenv("RELKIT_EXTENSION_ALLOWED"),
		"secretVisible": os.Getenv("RELKIT_EXTENSION_SECRET") != "",
	})
	os.Exit(0)
}

func TestSpecRejectsUnknownPermission(t *testing.T) {
	spec := Spec{
		Schema:       Schema,
		Kind:         KindArtifactProducer,
		Command:      []string{"producer"},
		StdinSchema:  "input/1",
		StdoutSchema: "output/1",
		Permissions:  []string{"rewrite-resolution"},
	}
	if err := spec.Validate(); err == nil {
		t.Fatal("unknown permission unexpectedly accepted")
	}
}

func ExampleSpec() {
	spec := Spec{
		Schema:       Schema,
		Kind:         KindArtifactProducer,
		Command:      []string{"./scripts/build"},
		StdinSchema:  "relkit.producer-input/1",
		StdoutSchema: "relkit.artifact-set/1",
	}
	fmt.Println(spec.Schema, spec.Kind)
	// Output: relkit.extension/1 artifact-producer
}
