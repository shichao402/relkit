package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/ops"
)

func TestEscapeAuditEmitsReviewAndReleaseReports(t *testing.T) {
	root := t.TempDir()
	registry := filepath.Join(root, "conformance", "production-escapes")
	if err := os.MkdirAll(registry, 0o755); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(root, "regression_test.go")
	if err := os.WriteFile(
		evidencePath,
		[]byte("package regression\n\nimport \"testing\"\n\nfunc TestCLIRegression(t *testing.T) {}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	crossComponent := false
	record := ops.ProductionEscape{
		Schema:            ops.ProductionEscapeSchema,
		Code:              "cli.review_lifecycle",
		Category:          ops.ProductionEscapeGeneric,
		Stage:             "audit",
		Capability:        "production-escape-audit",
		CrossComponent:    &crossComponent,
		Symptom:           "A review record needs coverage before its release is known.",
		WhyGateMissed:     "The old lifecycle required closure during review.",
		EarliestDetection: "Review audit checks executable evidence before merge.",
		Evidence: []ops.ProductionEscapeEvidence{{
			Path: "regression_test.go", Symbol: "TestCLIRegression", Kind: ops.ProductionEscapeEvidenceUnit,
		}},
		Tracking: &ops.ProductionEscapeTracking{
			Kind: ops.ProductionEscapeTrackingIssue, Number: 42,
		},
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(registry, "open.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	baseArgs := []string{
		"escape", "audit",
		"--registry", "conformance/production-escapes",
		"--repo-root", root,
		"--json",
	}
	output, err := captureOpsStdout(t, func() error {
		return run(context.Background(), baseArgs)
	})
	if err != nil {
		t.Fatalf("review audit: %v", err)
	}
	var review ops.ProductionEscapeAuditReport
	if err := json.Unmarshal(output, &review); err != nil {
		t.Fatalf("decode review report: %v\n%s", err, output)
	}
	if !review.OK || review.Mode != ops.ProductionEscapeAuditModeReview ||
		review.Covered != 1 || review.Open != 1 || review.Closed != 0 {
		t.Fatalf("review report=%+v", review)
	}

	releaseArgs := append([]string(nil), baseArgs...)
	releaseArgs = append(releaseArgs, "--release", "v1.0.0")
	output, err = captureOpsStdout(t, func() error {
		return run(context.Background(), releaseArgs)
	})
	if err == nil {
		t.Fatal("release audit accepted an open production escape")
	}
	var release ops.ProductionEscapeAuditReport
	if err := json.Unmarshal(output, &release); err != nil {
		t.Fatalf("decode release report: %v\n%s", err, output)
	}
	if release.OK || release.Mode != ops.ProductionEscapeAuditModeRelease ||
		release.Release != "v1.0.0" || release.Open != 1 ||
		!auditReportHasFinding(release, "production_escape.closure_required") {
		t.Fatalf("release report=%+v", release)
	}
}

func TestEmitCertificationReportJSONPreservesFailedExit(t *testing.T) {
	report := &ops.CertificationReport{
		Schema:          ops.CertificationReportSchema,
		CandidateDigest: "sha256:failed",
		Passed:          false,
	}
	output, err := captureOpsStdout(t, func() error {
		return emitCertificationReport(report, true)
	})
	if err == nil {
		t.Fatal("failed JSON certification report returned success")
	}
	var decoded ops.CertificationReport
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("decode certification report: %v\n%s", err, output)
	}
	if decoded.Passed {
		t.Fatalf("certification report=%+v", decoded)
	}
}

func TestEmitCertificationReportJSONReturnsSuccessForPassingReport(t *testing.T) {
	report := &ops.CertificationReport{
		Schema:          ops.CertificationReportSchema,
		CandidateDigest: "sha256:passed",
		Passed:          true,
	}
	output, err := captureOpsStdout(t, func() error {
		return emitCertificationReport(report, true)
	})
	if err != nil {
		t.Fatalf("passing JSON certification report: %v", err)
	}
	var decoded ops.CertificationReport
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("decode certification report: %v\n%s", err, output)
	}
	if !decoded.Passed {
		t.Fatalf("certification report=%+v", decoded)
	}
}

func captureOpsStdout(t *testing.T, call func() error) ([]byte, error) {
	t.Helper()
	output, err := os.CreateTemp(t.TempDir(), "stdout-*.json")
	if err != nil {
		t.Fatal(err)
	}
	path := output.Name()
	previous := os.Stdout
	os.Stdout = output
	callErr := call()
	os.Stdout = previous
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data, callErr
}

func auditReportHasFinding(report ops.ProductionEscapeAuditReport, code string) bool {
	for _, finding := range report.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
