package consume_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/consume"
	"github.com/shichao402/relkit/internal/testutil"
)

// stackCase mirrors conformance/consume/status-verify.json stack cases.
type stackCase struct {
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Tree             []string          `json:"tree"`
	Files            map[string]string `json:"files"`
	ExpectLanguages  []string          `json:"expectLanguages"`
	ExpectComponents []string          `json:"expectComponents"`
	ExpectTreeSHA256 string            `json:"expectTreeSHA256"`
}

type stackFixture struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Cases       []stackCase `json:"cases"`
}

func TestConsumeStackConformance(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testutil.ConformanceRoot(t), "consume", "status-verify.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture stackFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			root := t.TempDir()
			for _, rel := range tc.Tree {
				path := filepath.Join(root, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("signal\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for rel, content := range tc.Files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			if tc.ExpectTreeSHA256 != "" {
				got, err := consume.TreeSHA256(root)
				if err != nil {
					t.Fatalf("TreeSHA256: %v", err)
				}
				if got != tc.ExpectTreeSHA256 {
					t.Errorf("tree sha = %s, want %s", got, tc.ExpectTreeSHA256)
				}
				return
			}

			stack, err := consume.DetectStack(root)
			if err != nil {
				t.Fatalf("DetectStack: %v", err)
			}
			if strings.Join(stack.Languages, ",") != strings.Join(tc.ExpectLanguages, ",") {
				t.Errorf("languages = %v, want %v", stack.Languages, tc.ExpectLanguages)
			}
			components, err := consume.ConsumeComponents(root)
			if err != nil {
				t.Fatalf("ConsumeComponents: %v", err)
			}
			if strings.Join(components, ",") != strings.Join(tc.ExpectComponents, ",") {
				t.Errorf("components = %v, want %v", components, tc.ExpectComponents)
			}
		})
	}
}
