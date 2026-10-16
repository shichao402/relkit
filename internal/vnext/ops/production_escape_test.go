package ops

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/testutil"
)

func TestAuditProductionEscapeRegistryAcceptsValidRegistry(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	writeProductionEscapeRecord(t, registry, "valid.json", record)

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
	})
	if err != nil {
		t.Fatalf("audit: %v; findings=%+v", err, report.Findings)
	}
	if !report.OK || report.Schema != ProductionEscapeAuditReportSchema ||
		report.Mode != ProductionEscapeAuditModeReview ||
		report.Records != 1 || report.Covered != 1 ||
		report.Open != 0 || report.Closed != 1 || report.Evidence != 1 ||
		report.CoverageDigest == "" ||
		!slices.Equal(report.Codes, []string{record.Code}) {
		t.Fatalf("report=%+v", report)
	}
}

func TestAuditProductionEscapeRegistryAllowsCoveredOpenRecordDuringReview(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ProductionEscape)
	}{
		{
			name: "tracking issue before PR",
			mutate: func(record *ProductionEscape) {
				record.Fix = nil
				record.Tracking = &ProductionEscapeTracking{
					Kind: ProductionEscapeTrackingIssue, Number: 42,
				}
			},
		},
		{
			name: "fix PR before merge",
			mutate: func(record *ProductionEscape) {
				record.Fix = &ProductionEscapeFix{PR: intPointer(7)}
			},
		},
		{
			name: "merged commit before release",
			mutate: func(record *ProductionEscape) {
				record.Fix = &ProductionEscapeFix{
					PR: intPointer(7), Commit: "0123456789abcdef0123456789abcdef01234567",
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, registry, evidencePath := newProductionEscapeAuditFixture(t)
			record := validProductionEscape(evidencePath, "TestRegression")
			test.mutate(&record)
			writeProductionEscapeRecord(t, registry, "open.json", record)

			report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
				Registry: "conformance/production-escapes",
				RepoRoot: root,
			})
			if err != nil {
				t.Fatalf("review audit: %v; findings=%+v", err, report.Findings)
			}
			if report.Mode != ProductionEscapeAuditModeReview || report.Covered != 1 ||
				report.Open != 1 || report.Closed != 0 {
				t.Fatalf("review report=%+v", report)
			}
		})
	}
}

func TestAuditProductionEscapeRegistryRequiresClosureForRelease(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	tagProductionEscapeFixture(t, root, "v1.2.3")
	record := validProductionEscape(evidencePath, "TestRegression")
	record.Fix = &ProductionEscapeFix{PR: intPointer(7)}
	writeProductionEscapeRecord(t, registry, "open.json", record)

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
		Release:  "v1.2.3",
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.closure_required") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	if report.Mode != ProductionEscapeAuditModeRelease || report.Release != "v1.2.3" ||
		report.Covered != 1 || report.Open != 1 || report.Closed != 0 {
		t.Fatalf("release report=%+v", report)
	}
}

func TestAuditProductionEscapeRegistryAcceptsClosedRecordForRelease(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	closeProductionEscapeFixture(t, root, &record, "v1.2.3")
	writeProductionEscapeRecord(t, registry, "closed.json", record)
	runProductionEscapeGit(t, root, "add", "conformance/production-escapes/closed.json")
	runProductionEscapeGit(t, root, "commit", "-m", "close production escape")
	tagProductionEscapeFixture(t, root, "v1.2.3")

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
		Release:  "v1.2.3",
	})
	if err != nil {
		t.Fatalf("release audit: %v; findings=%+v", err, report.Findings)
	}
	if report.Covered != 1 || report.Open != 0 || report.Closed != 1 {
		t.Fatalf("release report=%+v", report)
	}
}

func TestAuditProductionEscapeRegistryRejectsClosureAfterAuditedRelease(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	tagProductionEscapeFixture(t, root, "v1.2.3")
	record := validProductionEscape(evidencePath, "TestRegression")
	record.Fix.Release = "v1.2.4"
	writeProductionEscapeRecord(t, registry, "future.json", record)

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
		Release:  "v1.2.3",
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.release_after_audit") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryRejectsFixOutsideDeclaredRelease(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	tagProductionEscapeFixture(t, root, "v1.2.3")
	record.Fix.Commit = strings.Repeat("a", 40)
	writeProductionEscapeRecord(t, registry, "closed.json", record)

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
		Release:  "v1.2.3",
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.fix_history_invalid") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryRejectsCheckoutDifferentFromReleaseTag(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	closeProductionEscapeFixture(t, root, &record, "v1.2.3")
	writeProductionEscapeRecord(t, registry, "closed.json", record)
	runProductionEscapeGit(t, root, "add", "conformance/production-escapes/closed.json")
	runProductionEscapeGit(t, root, "commit", "-m", "close production escape")
	tagProductionEscapeFixture(t, root, "v1.2.3")
	if err := os.WriteFile(filepath.Join(root, "later.txt"), []byte("later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runProductionEscapeGit(t, root, "add", "later.txt")
	runProductionEscapeGit(t, root, "commit", "-m", "move checkout past release")

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
		Release:  "v1.2.3",
	})
	if err == nil ||
		!hasProductionEscapeFinding(report, "production_escape.audit_release_source_mismatch") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryRejectsInvalidReleaseMode(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	writeProductionEscapeRecord(t, registry, "closed.json", record)

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
		Release:  "latest",
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.audit_release_invalid") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryRejectsPathEscape(t *testing.T) {
	root, registry, _ := newProductionEscapeAuditFixture(t)
	record := validProductionEscape("../outside_test.go", "TestRegression")
	writeProductionEscapeRecord(t, registry, "escape.json", record)

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.evidence_path_invalid") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryRejectsMissingSymbol(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestDoesNotExist")
	writeProductionEscapeRecord(t, registry, "missing-symbol.json", record)

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.evidence_symbol_missing") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryRejectsDuplicateCode(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	writeProductionEscapeRecord(t, registry, "first.json", record)
	writeProductionEscapeRecord(t, registry, "second.json", record)

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.code_duplicate") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryRequiresScenarioForGenericCrossComponent(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	crossComponent := true
	record.CrossComponent = &crossComponent
	writeProductionEscapeRecord(t, registry, "cross-component.json", record)

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.scenario_missing") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryRejectsUncoveredRegistry(t *testing.T) {
	root, _, _ := newProductionEscapeAuditFixture(t)
	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.registry_uncovered") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryRejectsUnknownAndIncompleteRecords(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(t *testing.T, registry string, record ProductionEscape)
		findingCode string
	}{
		{
			name: "unknown field",
			mutate: func(t *testing.T, registry string, record ProductionEscape) {
				data, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				var value map[string]any
				if err := json.Unmarshal(data, &value); err != nil {
					t.Fatal(err)
				}
				value["unknown"] = true
				writeProductionEscapeJSON(t, filepath.Join(registry, "unknown.json"), value)
			},
			findingCode: "production_escape.document_invalid",
		},
		{
			name: "empty field",
			mutate: func(t *testing.T, registry string, record ProductionEscape) {
				record.Symptom = " "
				writeProductionEscapeRecord(t, registry, "empty.json", record)
			},
			findingCode: "production_escape.field_empty",
		},
		{
			name: "no evidence",
			mutate: func(t *testing.T, registry string, record ProductionEscape) {
				record.Evidence = nil
				writeProductionEscapeRecord(t, registry, "uncovered.json", record)
			},
			findingCode: "production_escape.evidence_missing",
		},
		{
			name: "product category",
			mutate: func(t *testing.T, registry string, record ProductionEscape) {
				record.Category = "product"
				writeProductionEscapeRecord(t, registry, "product.json", record)
			},
			findingCode: "production_escape.category_invalid",
		},
		{
			name: "missing cross component classification",
			mutate: func(t *testing.T, registry string, record ProductionEscape) {
				record.CrossComponent = nil
				writeProductionEscapeRecord(t, registry, "missing-cross-component.json", record)
			},
			findingCode: "production_escape.field_invalid",
		},
		{
			name: "missing tracking and fix PR",
			mutate: func(t *testing.T, registry string, record ProductionEscape) {
				record.Tracking = nil
				record.Fix = nil
				writeProductionEscapeRecord(t, registry, "missing-tracking.json", record)
			},
			findingCode: "production_escape.tracking_missing",
		},
		{
			name: "release without commit",
			mutate: func(t *testing.T, registry string, record ProductionEscape) {
				record.Fix.Commit = ""
				writeProductionEscapeRecord(t, registry, "bad-closure.json", record)
			},
			findingCode: "production_escape.fix_invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, registry, evidencePath := newProductionEscapeAuditFixture(t)
			record := validProductionEscape(evidencePath, "TestRegression")
			test.mutate(t, registry, record)

			report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
				Registry: "conformance/production-escapes",
				RepoRoot: root,
			})
			if err == nil || !hasProductionEscapeFinding(report, test.findingCode) {
				t.Fatalf("err=%v report=%+v", err, report)
			}
		})
	}
}

func TestAuditProductionEscapeEvidenceRequiresExecutableTest(t *testing.T) {
	root := t.TempDir()
	root, err := canonicalDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "internal", "regression.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package regression\n\nfunc TestRegression() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	evidenceErr := auditProductionEscapeEvidence(root, ProductionEscapeEvidence{
		Path: "internal/regression.go", Symbol: "TestRegression", Kind: ProductionEscapeEvidenceUnit,
	})
	if evidenceErr == nil || evidenceErr.code != "production_escape.evidence_not_test" {
		t.Fatalf("evidence error=%+v", evidenceErr)
	}
}

func TestAuditProductionEscapeEvidenceSupportsCommonTestLayouts(t *testing.T) {
	tests := []struct {
		path   string
		symbol string
		data   string
	}{
		{
			path:   "pkg/regression_test.go",
			symbol: "TestRegression",
			data:   "package pkg\n\nimport \"testing\"\n\nfunc TestRegression(t *testing.T) {}\n",
		},
		{
			path:   "pkg/test_regression.py",
			symbol: "test_regression",
			data:   "def test_regression():\n    pass\n",
		},
		{
			path:   "tests/regression.rs",
			symbol: "regression",
			data:   "#[test]\nfn regression() {}\n",
		},
		{
			path:   "src/regression.test.ts",
			symbol: "regression works",
			data:   "test(\"regression works\", () => {});\n",
		},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			root := t.TempDir()
			root, err := canonicalDirectory(root)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, filepath.FromSlash(test.path))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(test.data), 0o644); err != nil {
				t.Fatal(err)
			}
			if filepath.Ext(test.path) == ".rs" {
				if err := os.WriteFile(
					filepath.Join(root, "Cargo.toml"),
					[]byte("[package]\nname = \"escape-fixture\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"),
					0o644,
				); err != nil {
					t.Fatal(err)
				}
			}
			if evidenceErr := auditProductionEscapeEvidence(root, ProductionEscapeEvidence{
				Path: test.path, Symbol: test.symbol, Kind: ProductionEscapeEvidenceUnit,
			}); evidenceErr != nil {
				t.Fatalf("evidence error=%+v", evidenceErr)
			}
		})
	}
}

func TestAuditProductionEscapeEvidenceRejectsSymbolMentionedOnlyInComment(t *testing.T) {
	root := t.TempDir()
	root, err := canonicalDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "regression_test.go")
	if err := os.WriteFile(
		path,
		[]byte("package regression\n\n// func TestRegression(t *testing.T) {}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	evidenceErr := auditProductionEscapeEvidence(root, ProductionEscapeEvidence{
		Path: "regression_test.go", Symbol: "TestRegression", Kind: ProductionEscapeEvidenceUnit,
	})
	if evidenceErr == nil || evidenceErr.code != "production_escape.evidence_not_test" {
		t.Fatalf("evidence error=%+v", evidenceErr)
	}
}

func TestGoProductionEscapeEvidenceRequiresUniqueRegisteredFile(t *testing.T) {
	root := t.TempDir()
	root, err := canonicalDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("package regression\n\nimport \"testing\"\n\nfunc TestRegression(t *testing.T) {}\n")
	for _, name := range []string{"registered_test.go", "alternate_test.go"} {
		if err := os.WriteFile(filepath.Join(root, name), source, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	evidenceErr := auditProductionEscapeEvidence(root, ProductionEscapeEvidence{
		Path: "registered_test.go", Symbol: "TestRegression", Kind: ProductionEscapeEvidenceUnit,
	})
	if evidenceErr == nil || evidenceErr.code != "production_escape.evidence_ownership_invalid" {
		t.Fatalf("evidence error=%+v", evidenceErr)
	}
}

func TestNodeProductionEscapeEvidenceRequiresRunnableTest(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{name: "test", data: `test("regression works", () => {})`, want: true},
		{name: "it", data: `it("regression works", () => {})`, want: true},
		{name: "skipped", data: `test.skip("regression works", () => {})`},
		{name: "todo", data: `it.todo("regression works", () => {})`},
		{name: "expected failure", data: `test.failing("regression works", () => {})`},
		{name: "suite", data: `describe("regression works", () => {})`},
		{name: "helper", data: `function regressionWorks() {}`},
		{name: "block comment", data: "/*\ntest(\"regression works\", () => {})\n*/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isExecutableTestEvidence(
				"test/regression.test.js",
				[]byte(test.data),
				"regression works",
			); got != test.want {
				t.Fatalf("isExecutableTestEvidence()=%v want=%v", got, test.want)
			}
		})
	}
}

func TestRustProductionEscapeEvidenceRejectsCommentOnlyTest(t *testing.T) {
	if isExecutableTestEvidence(
		"src/lib.rs",
		[]byte("// #[test]\n// fn regression() {}\n"),
		"regression",
	) {
		t.Fatal("Rust comment was accepted as an executable test")
	}
}

func TestPythonProductionEscapeExecutionRequiresTargetOK(t *testing.T) {
	if err := validatePythonEvidenceExecution(
		[]byte(`RELKIT_PYTHON_TEST_RESULT {"errors":0,"expectedFailures":0,"failures":0,"ok":true,"skipped":0,"symbol":"test_regression","testsRun":1,"unexpectedSuccesses":0}`+"\n"),
		"test_regression",
	); err != nil {
		t.Fatal(err)
	}
	for name, output := range map[string]string{
		"skipped":   `RELKIT_PYTHON_TEST_RESULT {"errors":0,"expectedFailures":0,"failures":0,"ok":false,"skipped":1,"symbol":"test_regression","testsRun":1,"unexpectedSuccesses":0}`,
		"unrelated": `RELKIT_PYTHON_TEST_RESULT {"errors":0,"expectedFailures":0,"failures":0,"ok":true,"skipped":0,"symbol":"test_other","testsRun":1,"unexpectedSuccesses":0}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePythonEvidenceExecution(
				[]byte(output),
				"test_regression",
			); err == nil {
				t.Fatal("execution output unexpectedly passed")
			}
		})
	}
}

func TestNodeProductionEscapeExecutionRequiresTargetPass(t *testing.T) {
	path := filepath.Join(t.TempDir(), "regression.test.mjs")
	if err := validateNodeEvidenceExecution(
		[]byte(`RELKIT_NODE_TEST_RESULT {"type":"test:pass","name":"regression works","kind":"test","file":"`+filepath.ToSlash(path)+`","skip":false,"todo":false}`+"\n"),
		"regression works",
		path,
	); err != nil {
		t.Fatal(err)
	}
	for name, output := range map[string]string{
		"skipped":    `RELKIT_NODE_TEST_RESULT {"type":"test:pass","name":"regression works","kind":"test","file":"` + filepath.ToSlash(path) + `","skip":true,"todo":false}`,
		"suite":      `RELKIT_NODE_TEST_RESULT {"type":"test:pass","name":"regression works","kind":"suite","file":"` + filepath.ToSlash(path) + `","skip":false,"todo":false}`,
		"wrong file": `RELKIT_NODE_TEST_RESULT {"type":"test:pass","name":"regression works","kind":"test","file":"/wrong.test.mjs","skip":false,"todo":false}`,
		"unrelated":  `RELKIT_NODE_TEST_RESULT {"type":"test:pass","name":"another regression","kind":"test","file":"` + filepath.ToSlash(path) + `","skip":false,"todo":false}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateNodeEvidenceExecution(
				[]byte(output),
				"regression works",
				path,
			); err == nil {
				t.Fatal("execution output unexpectedly passed")
			}
		})
	}
}

func TestPythonProductionEscapeRunnerRejectsDynamicSkip(t *testing.T) {
	if firstAvailableCommand("python3", "python") == "" {
		t.Skip("python is not available")
	}
	root := t.TempDir()
	path := filepath.Join(root, "test_regression.py")
	write := func(body string) {
		t.Helper()
		source := "import unittest\n\nclass Regression(unittest.TestCase):\n" +
			"    def test_regression(self):\n" + body
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	evidence := ProductionEscapeEvidence{
		Path: "test_regression.py", Symbol: "test_regression", Kind: ProductionEscapeEvidenceUnit,
	}
	write("        print('RELKIT_PYTHON_TEST_RESULT fake')\n")
	if err := executeProductionEscapeEvidence(root, evidence); err != nil {
		t.Fatal(err)
	}
	write("        self.skipTest('not covered')\n")
	if err := executeProductionEscapeEvidence(root, evidence); err == nil {
		t.Fatal("dynamically skipped Python test passed")
	}
	if err := os.WriteFile(
		path,
		[]byte(`
print('RELKIT_PYTHON_TEST_RESULT {"symbol":"test_regression","testsRun":1,"failures":0,"errors":0,"skipped":0,"expectedFailures":0,"unexpectedSuccesses":0,"ok":true}')
raise SystemExit(0)

import unittest
class Regression(unittest.TestCase):
    def test_regression(self):
        self.fail("unreachable")
`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := executeProductionEscapeEvidence(root, evidence); err == nil {
		t.Fatal("Python import output and early exit forged a pass")
	}
	if err := os.WriteFile(
		filepath.Join(root, "imported_case.py"),
		[]byte("import unittest\nclass Imported(unittest.TestCase):\n    def test_regression(self):\n        pass\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		path,
		[]byte("from imported_case import Imported\nclass Local(Imported):\n    pass\n\ndef test_regression():\n    pass\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := executeProductionEscapeEvidence(root, evidence); err == nil {
		t.Fatal("Python inherited test method from another file satisfied the evidence")
	}
}

func TestNodeProductionEscapeRunnerRejectsDynamicSkip(t *testing.T) {
	if firstAvailableCommand("node") == "" {
		t.Skip("node is not available")
	}
	root := t.TempDir()
	path := filepath.Join(root, "regression.test.mjs")
	write := func(body string) {
		t.Helper()
		source := "import test from 'node:test';\n" +
			"test('regression works', " + body + ");\n"
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	evidence := ProductionEscapeEvidence{
		Path: "regression.test.mjs", Symbol: "regression works", Kind: ProductionEscapeEvidenceUnit,
	}
	write("() => { console.log('RELKIT_NODE_TEST_RESULT fake'); }")
	if err := executeProductionEscapeEvidence(root, evidence); err != nil {
		t.Fatal(err)
	}
	write("(context) => { context.skip('not covered'); }")
	if err := executeProductionEscapeEvidence(root, evidence); err == nil {
		t.Fatal("dynamically skipped Node test passed")
	}
	if err := os.WriteFile(
		filepath.Join(root, "imported.test.mjs"),
		[]byte("import test from 'node:test';\ntest('regression works', () => {});\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		path,
		[]byte("import test from 'node:test';\nimport './imported.test.mjs';\nif (false) {\n  test('regression works', () => {});\n}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := executeProductionEscapeEvidence(root, evidence); err == nil {
		t.Fatal("Node test imported from another file satisfied the evidence")
	}
}

func TestRustProductionEscapeRunnerBindsIntegrationTarget(t *testing.T) {
	if firstAvailableCommand("cargo") == "" {
		t.Skip("cargo is not available")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "[package]\nname = \"escape-fixture\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"
	if err := os.WriteFile(
		filepath.Join(root, "Cargo.toml"),
		[]byte(manifest),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(root, "tests", "regression.rs")
	if err := os.WriteFile(
		evidencePath,
		[]byte("#[test]\n#[ignore]\nfn regression() {}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "tests", "alternate.rs"),
		[]byte("#[test]\nfn regression() {}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	ignored := ProductionEscapeEvidence{
		Path: "tests/regression.rs", Symbol: "regression", Kind: ProductionEscapeEvidenceUnit,
	}
	if err := verifyRustEvidenceOwnership(root, evidencePath, ignored.Symbol); err == nil {
		t.Fatal("ignored Rust declaration was accepted as owned evidence")
	}
	if err := executeProductionEscapeEvidence(root, ignored); err == nil {
		t.Fatal("same-name test in another Rust integration target replaced ignored evidence")
	}
	if err := os.WriteFile(evidencePath, []byte("#[test]\nfn regression() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "Cargo.toml"),
		[]byte(manifest+"\n[[test]]\nname = \"regression\"\npath = \"tests/alternate.rs\"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := verifyRustEvidenceOwnership(root, evidencePath, ignored.Symbol); err == nil {
		t.Fatal("Cargo target remapping away from evidence was accepted")
	}
	if err := executeProductionEscapeEvidence(root, ignored); err == nil {
		t.Fatal("Cargo target remapping executed alternate evidence")
	}
	if err := os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyRustEvidenceOwnership(root, evidencePath, ignored.Symbol); err != nil {
		t.Fatal(err)
	}
	if err := executeProductionEscapeEvidence(root, ignored); err != nil {
		t.Fatal(err)
	}
}

func TestAuditProductionEscapeRegistryExecutesRegressionEvidence(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	writeProductionEscapeRecord(t, registry, "valid.json", record)
	absoluteEvidence := filepath.Join(root, filepath.FromSlash(evidencePath))
	if err := os.WriteFile(
		absoluteEvidence,
		[]byte("package regression\n\nimport \"testing\"\n\nfunc TestRegression(t *testing.T) { t.Fatal(\"escaped\") }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
		Execute:  true,
	})
	if err == nil || !report.Execute || report.Executed != 1 ||
		!hasProductionEscapeFinding(report, "production_escape.evidence_execution_failed") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryRejectsSkippedRegressionEvidence(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	writeProductionEscapeRecord(t, registry, "valid.json", record)
	absoluteEvidence := filepath.Join(root, filepath.FromSlash(evidencePath))
	if err := os.WriteFile(
		absoluteEvidence,
		[]byte("package regression\n\nimport \"testing\"\n\nfunc TestRegression(t *testing.T) { t.Skip(\"not covered\") }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
		Execute:  true,
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.evidence_execution_failed") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryDefersEvidenceForOtherPlatforms(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	otherPlatform := "windows"
	if runtime.GOOS == "windows" {
		otherPlatform = "linux"
	}
	record.Evidence[0].Platforms = []string{otherPlatform}
	writeProductionEscapeRecord(t, registry, "valid.json", record)

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
		Execute:  true,
	})
	if err != nil || report.Executed != 0 || report.Deferred != 1 {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryDigestBindsEvidenceSource(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	writeProductionEscapeRecord(t, registry, "valid.json", record)
	first, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	absoluteEvidence := filepath.Join(root, filepath.FromSlash(evidencePath))
	if err := os.WriteFile(
		absoluteEvidence,
		[]byte("package regression\n\nimport \"testing\"\n\nfunc TestRegression(t *testing.T) { t.Log(\"changed\") }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	second, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.CoverageDigest == second.CoverageDigest {
		t.Fatalf("coverage digest did not change with test source: %s", first.CoverageDigest)
	}
}

func TestAuditProductionEscapeRegistryRejectsBaselineRecordDeletion(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	writeProductionEscapeRecord(t, registry, "existing.json", record)
	runProductionEscapeGit(t, root, "add", "conformance/production-escapes/existing.json")
	runProductionEscapeGit(t, root, "commit", "-m", "register production escape")
	baseline := strings.TrimSpace(runProductionEscapeGit(t, root, "rev-parse", "HEAD"))

	if err := os.Remove(filepath.Join(registry, "existing.json")); err != nil {
		t.Fatal(err)
	}
	replacement := record
	replacement.Code = "regression.other_escape"
	writeProductionEscapeRecord(t, registry, "other.json", replacement)

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry:    "conformance/production-escapes",
		RepoRoot:    root,
		BaselineRef: baseline,
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.record_removed") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryRequiresCleanGitSourceWhenRequested(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	writeProductionEscapeRecord(t, registry, "valid.json", record)

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry:         "conformance/production-escapes",
		RepoRoot:         root,
		RequireGitSource: true,
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.source_dirty") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	runProductionEscapeGit(t, root, "add", "conformance/production-escapes/valid.json")
	runProductionEscapeGit(t, root, "commit", "-m", "register production escape")
	report, err = AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry:         "conformance/production-escapes",
		RepoRoot:         root,
		RequireGitSource: true,
	})
	if err != nil || report.SourceDirty || report.SourceCommit == "" {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestAuditProductionEscapeRegistryRejectsIgnoredUntrackedInputs(t *testing.T) {
	root, registry, evidencePath := newProductionEscapeAuditFixture(t)
	record := validProductionEscape(evidencePath, "TestRegression")
	writeProductionEscapeRecord(t, registry, "ignored.json", record)
	excludePath := filepath.Join(root, ".git", "info", "exclude")
	exclude, err := os.OpenFile(excludePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exclude.WriteString("conformance/production-escapes/ignored.json\n"); err != nil {
		exclude.Close()
		t.Fatal(err)
	}
	if err := exclude.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry:         "conformance/production-escapes",
		RepoRoot:         root,
		RequireGitSource: true,
	})
	if err == nil || !hasProductionEscapeFinding(report, "production_escape.source_untracked") {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}

func TestRepositoryProductionEscapeRegistry(t *testing.T) {
	root := testutil.RepoRoot(t)
	report, err := AuditProductionEscapeRegistry(ProductionEscapeAuditOptions{
		Registry: "conformance/production-escapes",
		RepoRoot: root,
	})
	if err != nil {
		t.Fatalf("production escape registry: %v; findings=%+v", err, report.Findings)
	}
	if report.Mode != ProductionEscapeAuditModeReview || report.Covered != report.Records {
		t.Fatalf("repository registry is not covered: %+v", report)
	}
}

func newProductionEscapeAuditFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	registry := filepath.Join(root, "conformance", "production-escapes")
	if err := os.MkdirAll(registry, 0o755); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join("internal", "regression_test.go")
	absoluteEvidence := filepath.Join(root, evidencePath)
	if err := os.MkdirAll(filepath.Dir(absoluteEvidence), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		absoluteEvidence,
		[]byte("package regression\n\nimport \"testing\"\n\nfunc TestRegression(t *testing.T) {}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.invalid/production-escape-fixture\n\ngo 1.25\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runProductionEscapeGit(t, root, "init")
	runProductionEscapeGit(t, root, "config", "user.email", "test@example.invalid")
	runProductionEscapeGit(t, root, "config", "user.name", "Test")
	runProductionEscapeGit(t, root, "add", "go.mod", evidencePath)
	runProductionEscapeGit(t, root, "commit", "-m", "add regression")
	return root, registry, filepath.ToSlash(evidencePath)
}

func validProductionEscape(path, symbol string) ProductionEscape {
	crossComponent := false
	return ProductionEscape{
		Schema:            ProductionEscapeSchema,
		Code:              "regression.test_escape",
		Category:          ProductionEscapeGeneric,
		Stage:             "materialize",
		Capability:        "test-capability",
		CrossComponent:    &crossComponent,
		Symptom:           "A production path escaped the existing gate.",
		WhyGateMissed:     "The prior test did not exercise the production boundary.",
		EarliestDetection: "Run the regression before candidate certification.",
		Evidence: []ProductionEscapeEvidence{{
			Path: path, Symbol: symbol, Kind: ProductionEscapeEvidenceUnit,
		}},
		Fix: &ProductionEscapeFix{
			PR:      intPointer(1),
			Commit:  "0123456789abcdef0123456789abcdef01234567",
			Release: "v1.2.3",
		},
	}
}

func intPointer(value int) *int {
	return &value
}

func writeProductionEscapeRecord(t *testing.T, registry, name string, record ProductionEscape) {
	t.Helper()
	writeProductionEscapeJSON(t, filepath.Join(registry, name), record)
}

func writeProductionEscapeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasProductionEscapeFinding(report *ProductionEscapeAuditReport, code string) bool {
	for _, finding := range report.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func closeProductionEscapeFixture(t *testing.T, root string, record *ProductionEscape, release string) {
	t.Helper()
	record.Fix.Commit = strings.TrimSpace(runProductionEscapeGit(t, root, "rev-parse", "HEAD"))
	record.Fix.Release = release
}

func tagProductionEscapeFixture(t *testing.T, root, release string) {
	t.Helper()
	runProductionEscapeGit(t, root, "tag", release)
}

func runProductionEscapeGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
