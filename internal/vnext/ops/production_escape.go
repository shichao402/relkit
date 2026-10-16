package ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
)

const (
	ProductionEscapeSchema            = "relkit.production-escape/1"
	ProductionEscapeAuditReportSchema = "relkit.production-escape-audit-report/1"
	ProductionEscapeAuditModeReview   = "review"
	ProductionEscapeAuditModeRelease  = "release"
)

type ProductionEscapeCategory string

const (
	ProductionEscapeGeneric ProductionEscapeCategory = "generic"
	ProductionEscapeAgent   ProductionEscapeCategory = "agent"
)

type ProductionEscapeEvidenceKind string

const (
	ProductionEscapeEvidenceUnit     ProductionEscapeEvidenceKind = "unit"
	ProductionEscapeEvidenceScenario ProductionEscapeEvidenceKind = "scenario"
)

type ProductionEscapeEvidence struct {
	Path      string                       `json:"path"`
	Symbol    string                       `json:"symbol"`
	Kind      ProductionEscapeEvidenceKind `json:"kind"`
	Platforms []string                     `json:"platforms,omitempty"`
}

type ProductionEscapeTrackingKind string

const (
	ProductionEscapeTrackingIssue       ProductionEscapeTrackingKind = "issue"
	ProductionEscapeTrackingPullRequest ProductionEscapeTrackingKind = "pull-request"
)

type ProductionEscapeTracking struct {
	Kind   ProductionEscapeTrackingKind `json:"kind"`
	Number int                          `json:"number"`
}

type ProductionEscapeFix struct {
	PR      *int   `json:"pr,omitempty"`
	Commit  string `json:"commit,omitempty"`
	Release string `json:"release,omitempty"`
}

// ProductionEscape is the central, executable regression record. The pointer
// makes crossComponent a required JSON field even when its value is false.
type ProductionEscape struct {
	Schema            string                     `json:"schema"`
	Code              string                     `json:"code"`
	Category          ProductionEscapeCategory   `json:"category"`
	Stage             string                     `json:"stage"`
	Capability        string                     `json:"capability"`
	CrossComponent    *bool                      `json:"crossComponent"`
	Symptom           string                     `json:"symptom"`
	WhyGateMissed     string                     `json:"whyGateMissed"`
	EarliestDetection string                     `json:"earliestDetection"`
	Evidence          []ProductionEscapeEvidence `json:"evidence"`
	Tracking          *ProductionEscapeTracking  `json:"tracking,omitempty"`
	Fix               *ProductionEscapeFix       `json:"fix,omitempty"`
}

// productionEscapeCoverage intentionally excludes closure metadata. The same
// digest must survive the open -> closed transition so candidate certification
// remains bound to the regression that the release gate later closes.
type productionEscapeCoverage struct {
	Schema            string                             `json:"schema"`
	Code              string                             `json:"code"`
	Category          ProductionEscapeCategory           `json:"category"`
	Stage             string                             `json:"stage"`
	Capability        string                             `json:"capability"`
	CrossComponent    *bool                              `json:"crossComponent"`
	Symptom           string                             `json:"symptom"`
	WhyGateMissed     string                             `json:"whyGateMissed"`
	EarliestDetection string                             `json:"earliestDetection"`
	Evidence          []productionEscapeCoverageEvidence `json:"evidence"`
	Tracking          *ProductionEscapeTracking          `json:"tracking,omitempty"`
}

type productionEscapeCoverageEvidence struct {
	Path         string                       `json:"path"`
	Symbol       string                       `json:"symbol"`
	Kind         ProductionEscapeEvidenceKind `json:"kind"`
	Platforms    []string                     `json:"platforms,omitempty"`
	SourceDigest string                       `json:"sourceDigest"`
}

type ProductionEscapeAuditFinding struct {
	Code       string `json:"code"`
	Path       string `json:"path"`
	EscapeCode string `json:"escapeCode,omitempty"`
	Message    string `json:"message"`
}

type ProductionEscapeAuditReport struct {
	Schema         string                         `json:"schema"`
	OK             bool                           `json:"ok"`
	Mode           string                         `json:"mode"`
	Release        string                         `json:"release,omitempty"`
	Registry       string                         `json:"registry"`
	RepoRoot       string                         `json:"repoRoot"`
	BaselineRef    string                         `json:"baselineRef,omitempty"`
	CoverageDigest string                         `json:"coverageDigest,omitempty"`
	SourceCommit   string                         `json:"sourceCommit,omitempty"`
	SourceDirty    bool                           `json:"sourceDirty"`
	Execute        bool                           `json:"execute"`
	Executed       int                            `json:"executed"`
	Deferred       int                            `json:"deferred"`
	Files          []string                       `json:"files"`
	Records        int                            `json:"records"`
	Covered        int                            `json:"covered"`
	Open           int                            `json:"open"`
	Closed         int                            `json:"closed"`
	Evidence       int                            `json:"evidence"`
	Codes          []string                       `json:"codes"`
	Findings       []ProductionEscapeAuditFinding `json:"findings"`
}

type ProductionEscapeAuditOptions struct {
	Registry         string
	RepoRoot         string
	BaselineRef      string
	Release          string
	Execute          bool
	RequireGitSource bool
}

type ProductionEscapeAuditError struct {
	Findings int
}

func (e ProductionEscapeAuditError) Error() string {
	return fmt.Sprintf("production escape registry audit failed with %d finding(s)", e.Findings)
}

var (
	escapeIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
	escapeCommitPattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	escapeReleasePattern    = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
	pythonTestDeclPattern   = regexp.MustCompile(`(?m)^[[:space:]]*(?:async[[:space:]]+)?def[[:space:]]+([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*\(`)
	nodeTestCallPattern     = regexp.MustCompile(`(?m)^[[:space:]]*(test|it)(?:\.([A-Za-z]+))?[[:space:]]*\([[:space:]]*["'\x60]([^"'\x60]+)["'\x60]`)
)

const pythonProductionEscapeRunner = `
import contextlib
import importlib.util
import io
import json
import pathlib
import sys
import unittest

path = pathlib.Path(sys.argv[1]).resolve()
symbol = sys.argv[2]
sys.path.insert(0, str(path.parent))
def flatten(suite):
    for item in suite:
        if isinstance(item, unittest.TestSuite):
            yield from flatten(item)
        else:
            yield item
payload = {
    "symbol": symbol,
    "testsRun": 0,
    "failures": 0,
    "errors": 1,
    "skipped": 0,
    "expectedFailures": 0,
    "unexpectedSuccesses": 0,
}
try:
    with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
        spec = importlib.util.spec_from_file_location("_relkit_production_escape", path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        loaded = unittest.defaultTestLoader.loadTestsFromModule(module)
        selected = [
            test for test in flatten(loaded)
            if test.__class__.__module__ == module.__name__
            and pathlib.Path(
                getattr(test.__class__, test._testMethodName).__code__.co_filename
            ).resolve() == path
            and (test.id() == symbol or test.id().endswith("." + symbol))
        ]
        suite = unittest.TestSuite(selected)
        result = unittest.TestResult()
        suite.run(result)
    payload.update({
        "testsRun": result.testsRun,
        "failures": len(result.failures),
        "errors": len(result.errors),
        "skipped": len(result.skipped),
        "expectedFailures": len(result.expectedFailures),
        "unexpectedSuccesses": len(result.unexpectedSuccesses),
    })
except BaseException:
    pass
payload["ok"] = (
    payload["testsRun"] == 1
    and payload["failures"] == 0
    and payload["errors"] == 0
    and payload["skipped"] == 0
    and payload["expectedFailures"] == 0
    and payload["unexpectedSuccesses"] == 0
)
print("RELKIT_PYTHON_TEST_RESULT " + json.dumps(payload, sort_keys=True))
raise SystemExit(0 if payload["ok"] else 3)
`

const nodeProductionEscapeReporter = `
export default async function* (source) {
  for await (const event of source) {
    if (event.type !== "test:pass" && event.type !== "test:fail") {
      continue;
    }
    const details = event.data?.details ?? {};
    yield "RELKIT_NODE_TEST_RESULT " + JSON.stringify({
      type: event.type,
      name: event.data?.name ?? "",
      kind: details.type ?? "",
      file: event.data?.file ?? details.file ?? "",
      skip: Boolean(event.data?.skip ?? details.skip),
      todo: Boolean(event.data?.todo ?? details.todo)
    }) + "\n";
  }
}
`

// AuditProductionEscapeRegistry validates every direct *.json child of the
// registry. It never consults the legacy journal or DIGESTED_ISSUE_CODES:
// executable regression evidence in the repository is the only coverage.
func AuditProductionEscapeRegistry(options ProductionEscapeAuditOptions) (*ProductionEscapeAuditReport, error) {
	mode := ProductionEscapeAuditModeReview
	if options.Release != "" {
		mode = ProductionEscapeAuditModeRelease
	}
	report := &ProductionEscapeAuditReport{
		Schema:      ProductionEscapeAuditReportSchema,
		Mode:        mode,
		Release:     options.Release,
		Registry:    filepath.ToSlash(filepath.Clean(options.Registry)),
		RepoRoot:    filepath.ToSlash(filepath.Clean(options.RepoRoot)),
		BaselineRef: options.BaselineRef,
		Execute:     options.Execute,
		Files:       []string{},
		Codes:       []string{},
		Findings:    []ProductionEscapeAuditFinding{},
	}
	addFinding := func(code, path, escapeCode, message string) {
		report.Findings = append(report.Findings, ProductionEscapeAuditFinding{
			Code: code, Path: filepath.ToSlash(path), EscapeCode: escapeCode, Message: message,
		})
	}
	releaseValid := true
	if mode == ProductionEscapeAuditModeRelease {
		if !escapeReleasePattern.MatchString(options.Release) {
			addFinding(
				"production_escape.audit_release_invalid",
				options.Release,
				"",
				"--release must be a v-prefixed semantic version",
			)
			releaseValid = false
		} else if _, compareErr := catalog.CompareVersions(options.Release, options.Release); compareErr != nil {
			addFinding("production_escape.audit_release_invalid", options.Release, "", compareErr.Error())
			releaseValid = false
		}
	}

	repoRoot, err := canonicalDirectory(options.RepoRoot)
	if err != nil {
		addFinding("production_escape.repo_root_invalid", options.RepoRoot, "", err.Error())
		return finishProductionEscapeAudit(report)
	}
	if mode == ProductionEscapeAuditModeRelease && releaseValid {
		if err := requireGitCommit(repoRoot, "refs/tags/"+options.Release); err != nil {
			addFinding(
				"production_escape.audit_release_missing",
				options.Release,
				"",
				fmt.Sprintf("audited release tag is not available as a Git commit: %v", err),
			)
			releaseValid = false
		}
		if releaseValid && options.BaselineRef != "" {
			if err := requireGitAncestor(
				repoRoot,
				options.BaselineRef,
				"refs/tags/"+options.Release,
			); err != nil {
				addFinding(
					"production_escape.baseline_invalid",
					options.BaselineRef,
					"",
					fmt.Sprintf("release baseline must be an ancestor of %s: %v", options.Release, err),
				)
				releaseValid = false
			}
		}
	}
	registry, err := resolveRegistryDirectory(repoRoot, options.Registry)
	if err != nil {
		addFinding("production_escape.registry_invalid", options.Registry, "", err.Error())
		return finishProductionEscapeAudit(report)
	}
	entries, err := os.ReadDir(registry)
	if err != nil {
		addFinding("production_escape.registry_invalid", options.Registry, "", err.Error())
		return finishProductionEscapeAudit(report)
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			report.Files = append(report.Files, entry.Name())
		}
	}
	sort.Strings(report.Files)
	if len(report.Files) == 0 {
		addFinding(
			"production_escape.registry_uncovered",
			options.Registry,
			"",
			"registry contains no production escape JSON records",
		)
		return finishProductionEscapeAudit(report)
	}

	seenCodes := map[string]string{}
	recordsByCode := map[string]ProductionEscape{}
	executedEvidence := map[string]bool{}
	evidenceSourceDigests := map[string]string{}
	for _, name := range report.Files {
		recordPath := filepath.Join(registry, name)
		info, statErr := os.Lstat(recordPath)
		if statErr != nil {
			addFinding("production_escape.document_invalid", name, "", statErr.Error())
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			addFinding("production_escape.document_invalid", name, "", "registry record must be a regular file, not a symlink")
			continue
		}

		var record ProductionEscape
		if loadErr := document.Load(recordPath, &record); loadErr != nil {
			addFinding("production_escape.document_invalid", name, "", loadErr.Error())
			continue
		}
		report.Records++
		report.Evidence += len(record.Evidence)

		recordFindings := validateProductionEscape(record, name)
		if record.Code != "" {
			if previous, duplicate := seenCodes[record.Code]; duplicate {
				recordFindings = append(recordFindings, ProductionEscapeAuditFinding{
					Code:       "production_escape.code_duplicate",
					Path:       filepath.ToSlash(name),
					EscapeCode: record.Code,
					Message:    fmt.Sprintf("code duplicates %s", previous),
				})
			} else {
				seenCodes[record.Code] = name
				recordsByCode[record.Code] = record
				report.Codes = append(report.Codes, record.Code)
			}
		}
		for _, evidence := range record.Evidence {
			if evidence.Path == "" || evidence.Symbol == "" {
				continue
			}
			evidenceErr := auditProductionEscapeEvidence(repoRoot, evidence)
			if evidenceErr != nil {
				recordFindings = append(recordFindings, ProductionEscapeAuditFinding{
					Code:       evidenceErr.code,
					Path:       filepath.ToSlash(evidence.Path),
					EscapeCode: record.Code,
					Message:    evidenceErr.message,
				})
			} else if options.Execute {
				key := evidence.Path + "\x00" + evidence.Symbol + "\x00" + strings.Join(evidence.Platforms, ",")
				if !executedEvidence[key] {
					executedEvidence[key] = true
					if !productionEscapeEvidenceApplies(evidence, runtime.GOOS) {
						report.Deferred++
					} else if executionErr := executeProductionEscapeEvidence(repoRoot, evidence); executionErr != nil {
						report.Executed++
						recordFindings = append(recordFindings, ProductionEscapeAuditFinding{
							Code:       "production_escape.evidence_execution_failed",
							Path:       filepath.ToSlash(evidence.Path),
							EscapeCode: record.Code,
							Message:    executionErr.Error(),
						})
					} else {
						report.Executed++
					}
				}
			}
			if evidenceErr == nil {
				digest, digestErr := productionEscapeEvidenceSourceDigest(repoRoot, evidence.Path)
				if digestErr != nil {
					recordFindings = append(recordFindings, ProductionEscapeAuditFinding{
						Code:       "production_escape.evidence_digest_failed",
						Path:       filepath.ToSlash(evidence.Path),
						EscapeCode: record.Code,
						Message:    digestErr.Error(),
					})
				} else {
					evidenceSourceDigests[evidence.Path] = digest
				}
			}
		}
		report.Findings = append(report.Findings, recordFindings...)
		if len(recordFindings) != 0 {
			continue
		}
		report.Covered++
		if productionEscapeClosed(record) {
			report.Closed++
			if mode == ProductionEscapeAuditModeRelease && releaseValid {
				comparison, compareErr := catalog.CompareVersions(record.Fix.Release, options.Release)
				if compareErr != nil {
					addFinding("production_escape.fix_invalid", name, record.Code, compareErr.Error())
				} else if comparison > 0 {
					addFinding(
						"production_escape.release_after_audit",
						name,
						record.Code,
						fmt.Sprintf("fix.release %s is after audited release %s", record.Fix.Release, options.Release),
					)
				}
				if historyErr := verifyProductionEscapeFixHistory(repoRoot, *record.Fix, options.Release); historyErr != nil {
					addFinding(
						"production_escape.fix_history_invalid",
						name,
						record.Code,
						historyErr.Error(),
					)
				}
			}
		} else {
			report.Open++
			if mode == ProductionEscapeAuditModeRelease {
				addFinding(
					"production_escape.closure_required",
					name,
					record.Code,
					fmt.Sprintf("release %s requires fix.pr, full fix.commit, and fix.release", options.Release),
				)
			}
		}
	}
	sort.Strings(report.Codes)
	if len(recordsByCode) > 0 {
		coverage := make([]productionEscapeCoverage, 0, len(recordsByCode))
		for _, code := range report.Codes {
			record, ok := recordsByCode[code]
			if !ok {
				continue
			}
			evidence := make([]productionEscapeCoverageEvidence, 0, len(record.Evidence))
			for _, item := range record.Evidence {
				evidence = append(evidence, productionEscapeCoverageEvidence{
					Path:         item.Path,
					Symbol:       item.Symbol,
					Kind:         item.Kind,
					Platforms:    item.Platforms,
					SourceDigest: evidenceSourceDigests[item.Path],
				})
			}
			coverage = append(coverage, productionEscapeCoverage{
				Schema:            record.Schema,
				Code:              record.Code,
				Category:          record.Category,
				Stage:             record.Stage,
				Capability:        record.Capability,
				CrossComponent:    record.CrossComponent,
				Symptom:           record.Symptom,
				WhyGateMissed:     record.WhyGateMissed,
				EarliestDetection: record.EarliestDetection,
				Evidence:          evidence,
				Tracking:          record.Tracking,
			})
		}
		digest, digestErr := canonical.Digest(coverage)
		if digestErr != nil {
			addFinding("production_escape.registry_digest_invalid", options.Registry, "", digestErr.Error())
		} else {
			report.CoverageDigest = digest
		}
	}
	sourceCommit, sourceDirty, sourceErr := productionEscapeGitSourceState(repoRoot)
	if sourceErr != nil {
		if options.RequireGitSource {
			addFinding("production_escape.source_invalid", options.RepoRoot, "", sourceErr.Error())
		}
	} else {
		report.SourceCommit = sourceCommit
		report.SourceDirty = sourceDirty
		if options.RequireGitSource && sourceDirty {
			addFinding(
				"production_escape.source_dirty",
				options.RepoRoot,
				"",
				"the repository must be clean so candidate evidence matches the checked-out Git commit",
			)
		}
		if mode == ProductionEscapeAuditModeRelease && releaseValid {
			releaseCommit, resolveErr := resolveGitCommit(repoRoot, "refs/tags/"+options.Release)
			switch {
			case resolveErr != nil:
				addFinding("production_escape.audit_release_missing", options.Release, "", resolveErr.Error())
			case sourceCommit != releaseCommit:
				addFinding(
					"production_escape.audit_release_source_mismatch",
					options.Release,
					"",
					fmt.Sprintf("checked-out source %s does not match release tag commit %s", sourceCommit, releaseCommit),
				)
			case sourceDirty:
				addFinding(
					"production_escape.source_dirty",
					options.RepoRoot,
					"",
					"release audit requires registry and regression evidence from the clean tagged source",
				)
			}
		}
	}
	if options.RequireGitSource {
		for _, finding := range verifyProductionEscapeSourcesAtHEAD(
			repoRoot,
			registry,
			report.Files,
			recordsByCode,
		) {
			report.Findings = append(report.Findings, finding)
		}
	}
	if options.BaselineRef != "" {
		for _, finding := range auditProductionEscapeBaseline(
			repoRoot,
			registry,
			options.BaselineRef,
			recordsByCode,
		) {
			report.Findings = append(report.Findings, finding)
		}
	}
	return finishProductionEscapeAudit(report)
}

type productionEscapeEvidenceError struct {
	code    string
	message string
}

func validateProductionEscape(record ProductionEscape, path string) []ProductionEscapeAuditFinding {
	findings := []ProductionEscapeAuditFinding{}
	add := func(code, message string) {
		findings = append(findings, ProductionEscapeAuditFinding{
			Code: code, Path: filepath.ToSlash(path), EscapeCode: record.Code, Message: message,
		})
	}
	if record.Schema != ProductionEscapeSchema {
		add("production_escape.schema_invalid", fmt.Sprintf("schema is %q, expected %q", record.Schema, ProductionEscapeSchema))
	}
	for field, value := range map[string]string{
		"code":       record.Code,
		"stage":      record.Stage,
		"capability": record.Capability,
	} {
		if !escapeIdentifierPattern.MatchString(value) {
			add("production_escape.field_invalid", field+" must be a non-empty stable lowercase identifier")
		}
	}
	switch record.Category {
	case ProductionEscapeGeneric, ProductionEscapeAgent:
	default:
		add("production_escape.category_invalid", `category must be "generic" or "agent"; product-specific escapes stay in the host repository`)
	}
	if record.CrossComponent == nil {
		add("production_escape.field_invalid", "crossComponent is required")
	}
	for field, value := range map[string]string{
		"symptom":           record.Symptom,
		"whyGateMissed":     record.WhyGateMissed,
		"earliestDetection": record.EarliestDetection,
	} {
		if strings.TrimSpace(value) == "" {
			add("production_escape.field_empty", field+" must not be empty")
		}
	}
	if len(record.Evidence) == 0 {
		add("production_escape.evidence_missing", "at least one executable regression evidence entry is required")
	}
	hasScenario := false
	seenEvidence := map[string]bool{}
	for index, evidence := range record.Evidence {
		prefix := fmt.Sprintf("evidence[%d]", index)
		if strings.TrimSpace(evidence.Path) == "" {
			add("production_escape.field_empty", prefix+".path must not be empty")
		}
		if strings.TrimSpace(evidence.Symbol) == "" {
			add("production_escape.field_empty", prefix+".symbol must not be empty")
		}
		switch evidence.Kind {
		case ProductionEscapeEvidenceUnit:
		case ProductionEscapeEvidenceScenario:
			hasScenario = true
		default:
			add("production_escape.evidence_kind_invalid", prefix+`.kind must be "unit" or "scenario"`)
		}
		previousPlatform := ""
		for platformIndex, platform := range evidence.Platforms {
			switch platform {
			case "darwin", "linux", "windows":
			default:
				add(
					"production_escape.evidence_platform_invalid",
					fmt.Sprintf("%s.platforms[%d] must be darwin, linux, or windows", prefix, platformIndex),
				)
			}
			if previousPlatform != "" && platform <= previousPlatform {
				add(
					"production_escape.evidence_platform_invalid",
					prefix+".platforms must be sorted and contain no duplicates",
				)
			}
			previousPlatform = platform
		}
		key := evidence.Path + "\x00" + evidence.Symbol + "\x00" + string(evidence.Kind)
		if seenEvidence[key] {
			add("production_escape.evidence_duplicate", prefix+" duplicates an earlier evidence entry")
		}
		seenEvidence[key] = true
	}
	if record.CrossComponent != nil && *record.CrossComponent &&
		record.Category == ProductionEscapeGeneric && !hasScenario {
		add("production_escape.scenario_missing", "generic cross-component escapes require scenario evidence")
	}
	if record.Tracking != nil {
		switch record.Tracking.Kind {
		case ProductionEscapeTrackingIssue, ProductionEscapeTrackingPullRequest:
		default:
			add("production_escape.tracking_invalid", `tracking.kind must be "issue" or "pull-request"`)
		}
		if record.Tracking.Number <= 0 {
			add("production_escape.tracking_invalid", "tracking.number must be positive")
		}
	}
	var fixPR *int
	if record.Fix != nil {
		fixPR = record.Fix.PR
		if fixPR != nil && *fixPR <= 0 {
			add("production_escape.fix_invalid", "fix.pr must be positive when present")
		}
		if record.Fix.Commit != "" {
			if fixPR == nil {
				add("production_escape.fix_invalid", "fix.commit requires fix.pr")
			}
			if !escapeCommitPattern.MatchString(record.Fix.Commit) {
				add("production_escape.fix_invalid", "fix.commit must be a full lowercase 40-character Git commit")
			}
		}
		if record.Fix.Release != "" {
			if fixPR == nil || record.Fix.Commit == "" {
				add("production_escape.fix_invalid", "fix.release requires fix.pr and fix.commit")
			}
			if !escapeReleasePattern.MatchString(record.Fix.Release) {
				add("production_escape.fix_invalid", "fix.release must be a v-prefixed semantic version")
			} else if _, err := catalog.CompareVersions(record.Fix.Release, record.Fix.Release); err != nil {
				add("production_escape.fix_invalid", err.Error())
			}
		}
	}
	if record.Tracking == nil && fixPR == nil {
		add("production_escape.tracking_missing", "tracking issue/pull-request or fix.pr is required")
	}
	if record.Tracking != nil && record.Tracking.Kind == ProductionEscapeTrackingPullRequest &&
		fixPR != nil && record.Tracking.Number != *fixPR {
		add("production_escape.tracking_invalid", "pull-request tracking.number must match fix.pr")
	}
	return findings
}

func productionEscapeClosed(record ProductionEscape) bool {
	return record.Fix != nil && record.Fix.PR != nil &&
		record.Fix.Commit != "" && record.Fix.Release != ""
}

func auditProductionEscapeEvidence(repoRoot string, evidence ProductionEscapeEvidence) *productionEscapeEvidenceError {
	if filepath.IsAbs(evidence.Path) || filepath.VolumeName(evidence.Path) != "" ||
		isPortableWindowsAbsolute(evidence.Path) || strings.Contains(evidence.Path, `\`) {
		return &productionEscapeEvidenceError{
			code:    "production_escape.evidence_path_invalid",
			message: "evidence path must be a portable repo-relative slash path",
		}
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(evidence.Path)))
	if clean == "." || clean != evidence.Path || clean == ".." || strings.HasPrefix(clean, "../") {
		return &productionEscapeEvidenceError{
			code:    "production_escape.evidence_path_invalid",
			message: "evidence path escapes the repository or is not canonical",
		}
	}
	path := filepath.Join(repoRoot, filepath.FromSlash(clean))
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return &productionEscapeEvidenceError{
			code:    "production_escape.evidence_path_invalid",
			message: fmt.Sprintf("evidence path does not resolve to a repository file: %v", err),
		}
	}
	if !pathWithin(repoRoot, resolved) {
		return &productionEscapeEvidenceError{
			code:    "production_escape.evidence_path_invalid",
			message: "evidence path resolves outside the repository",
		}
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return &productionEscapeEvidenceError{
			code:    "production_escape.evidence_path_invalid",
			message: "evidence path must resolve to a regular repository file",
		}
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return &productionEscapeEvidenceError{
			code:    "production_escape.evidence_path_invalid",
			message: fmt.Sprintf("read evidence: %v", err),
		}
	}
	leaf := evidence.Symbol
	for _, separator := range []string{"::", ".", "/"} {
		if index := strings.LastIndex(leaf, separator); index >= 0 {
			leaf = leaf[index+len(separator):]
		}
	}
	symbolPresent := bytes.Contains(data, []byte(evidence.Symbol))
	switch strings.ToLower(filepath.Ext(clean)) {
	case ".py", ".rs":
		symbolPresent = symbolPresent || bytes.Contains(data, []byte(leaf))
	}
	if !symbolPresent {
		return &productionEscapeEvidenceError{
			code:    "production_escape.evidence_symbol_missing",
			message: fmt.Sprintf("evidence file does not contain symbol %q", evidence.Symbol),
		}
	}
	if !isExecutableTestEvidence(clean, data, evidence.Symbol) {
		return &productionEscapeEvidenceError{
			code: "production_escape.evidence_not_test",
			message: "evidence must resolve to an executable Go, Python, Rust, or Node test " +
				"(recognized test path or in-file test declaration)",
		}
	}
	switch strings.ToLower(filepath.Ext(clean)) {
	case ".go":
		if err := verifyGoEvidenceOwnership(repoRoot, resolved, evidence.Symbol); err != nil {
			return &productionEscapeEvidenceError{
				code:    "production_escape.evidence_ownership_invalid",
				message: err.Error(),
			}
		}
	case ".rs":
		if err := verifyRustEvidenceOwnership(repoRoot, resolved, evidence.Symbol); err != nil {
			return &productionEscapeEvidenceError{
				code:    "production_escape.evidence_ownership_invalid",
				message: err.Error(),
			}
		}
	}
	return nil
}

func isExecutableTestEvidence(path string, data []byte, symbol string) bool {
	lowerPath := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(lowerPath)
	extension := strings.ToLower(filepath.Ext(base))
	inTestDirectory := false
	for _, part := range strings.Split(lowerPath, "/") {
		switch part {
		case "test", "tests", "__tests__", "spec", "specs":
			inTestDirectory = true
		}
	}
	leaf := symbol
	for _, separator := range []string{"::", ".", "/"} {
		if index := strings.LastIndex(leaf, separator); index >= 0 {
			leaf = leaf[index+len(separator):]
		}
	}
	switch extension {
	case ".go":
		return strings.HasSuffix(base, "_test.go") && hasGoTestFunction(path, data, leaf)
	case ".py":
		named := strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") || inTestDirectory
		return named && regexpCaptureContains(pythonTestDeclPattern, data, leaf)
	case ".rs":
		return strictRustTestDeclarations(data, leaf) == 1
	case ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts":
		named := strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") || inTestDirectory
		if !named {
			return false
		}
		return hasRunnableNodeTest(stripJavaScriptComments(data), symbol)
	default:
		return false
	}
}

func hasGoTestFunction(path string, data []byte, name string) bool {
	file, err := parser.ParseFile(token.NewFileSet(), path, data, 0)
	if err != nil {
		return false
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || function.Name.Name != name ||
			function.Type.Results != nil || function.Type.Params == nil ||
			len(function.Type.Params.List) != 1 {
			continue
		}
		parameter, ok := function.Type.Params.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		testingType, ok := parameter.X.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		testingPackage, ok := testingType.X.(*ast.Ident)
		if !ok || testingPackage.Name != "testing" {
			continue
		}
		switch testingType.Sel.Name {
		case "T":
			if strings.HasPrefix(name, "Test") {
				return true
			}
		case "B":
			if strings.HasPrefix(name, "Benchmark") {
				return true
			}
		case "F":
			if strings.HasPrefix(name, "Fuzz") {
				return true
			}
		}
	}
	return false
}

func verifyGoEvidenceOwnership(repoRoot, evidencePath, symbol string) error {
	leaf := symbol
	for _, separator := range []string{"::", ".", "/"} {
		if index := strings.LastIndex(leaf, separator); index >= 0 {
			leaf = leaf[index+len(separator):]
		}
	}
	count := 0
	owned := false
	entries, err := os.ReadDir(filepath.Dir(evidencePath))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 ||
			!strings.HasSuffix(strings.ToLower(entry.Name()), "_test.go") {
			continue
		}
		path := filepath.Join(filepath.Dir(evidencePath), entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !hasGoTestFunction(path, data, leaf) {
			continue
		}
		count++
		if sameFilePath(path, evidencePath) {
			owned = true
		}
	}
	if count != 1 || !owned {
		return fmt.Errorf(
			"Go test %s must have exactly one package declaration in the registered evidence file (found %d)",
			symbol,
			count,
		)
	}
	if !pathWithin(repoRoot, evidencePath) {
		return fmt.Errorf("Go evidence path resolves outside the repository")
	}
	return nil
}

func verifyRustEvidenceOwnership(repoRoot, evidencePath, symbol string) error {
	if strings.Contains(symbol, "::") || strings.ContainsAny(symbol, "./") {
		return fmt.Errorf("Rust production escape symbols must be top-level integration test names")
	}
	manifest, err := findParentFile(repoRoot, filepath.Dir(evidencePath), "Cargo.toml")
	if err != nil {
		return err
	}
	crateRoot := filepath.Dir(manifest)
	integrationDirectory := filepath.Join(crateRoot, "tests")
	if !sameFilePath(filepath.Dir(evidencePath), integrationDirectory) {
		return fmt.Errorf("Rust production escape evidence must be a direct tests/*.rs integration target")
	}
	if _, err := rustIntegrationTarget(manifest, evidencePath); err != nil {
		return err
	}
	data, err := os.ReadFile(evidencePath)
	if err != nil {
		return err
	}
	count := strictRustTestDeclarations(data, symbol)
	if count != 1 {
		return fmt.Errorf(
			"Rust test %s must have exactly one unconditional top-level #[test] declaration in the registered integration file (found %d)",
			symbol,
			count,
		)
	}
	return nil
}

type rustTestTarget struct {
	name string
	path string
}

func rustIntegrationTarget(manifestPath, evidencePath string) (string, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}
	var targets []rustTestTarget
	var current *rustTestTarget
	inPackage := false
	autotests := true
	flush := func() {
		if current != nil {
			targets = append(targets, *current)
			current = nil
		}
	}
	for _, rawLine := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(stripTOMLComment(rawLine))
		switch {
		case line == "[[test]]":
			flush()
			current = &rustTestTarget{}
			inPackage = false
			continue
		case strings.HasPrefix(line, "["):
			flush()
			inPackage = line == "[package]"
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if current != nil {
			switch key {
			case "name":
				current.name = unquoteTOMLString(value)
			case "path":
				current.path = unquoteTOMLString(value)
			}
		} else if inPackage && key == "autotests" && value == "false" {
			autotests = false
		}
	}
	flush()

	manifestDirectory := filepath.Dir(manifestPath)
	matchedTarget := ""
	defaultName := strings.TrimSuffix(filepath.Base(evidencePath), filepath.Ext(evidencePath))
	for _, target := range targets {
		if target.name == "" {
			return "", fmt.Errorf("Cargo [[test]] target has no literal name")
		}
		targetPath := target.path
		if targetPath == "" {
			targetPath = filepath.Join("tests", target.name+".rs")
		}
		targetPath = filepath.Join(manifestDirectory, filepath.FromSlash(targetPath))
		if sameFilePath(targetPath, evidencePath) {
			if matchedTarget != "" {
				return "", fmt.Errorf("multiple Cargo test targets map to evidence file %s", evidencePath)
			}
			matchedTarget = target.name
		} else if target.name == defaultName {
			return "", fmt.Errorf(
				"Cargo test target %s maps to %s instead of evidence file %s",
				target.name,
				targetPath,
				evidencePath,
			)
		}
	}
	if matchedTarget != "" {
		return matchedTarget, nil
	}
	if !autotests {
		return "", fmt.Errorf("Cargo autotests are disabled and no explicit target maps to %s", evidencePath)
	}
	return defaultName, nil
}

func stripTOMLComment(line string) string {
	var quote byte
	escaped := false
	for index := 0; index < len(line); index++ {
		switch {
		case quote != 0:
			if escaped {
				escaped = false
			} else if line[index] == '\\' && quote == '"' {
				escaped = true
			} else if line[index] == quote {
				quote = 0
			}
		case line[index] == '"' || line[index] == '\'':
			quote = line[index]
		case line[index] == '#':
			return line[:index]
		}
	}
	return line
}

func unquoteTOMLString(value string) string {
	if len(value) < 2 {
		return ""
	}
	if (value[0] == '"' && value[len(value)-1] == '"') ||
		(value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return ""
}

func strictRustTestDeclarations(data []byte, name string) int {
	lines := strings.Split(string(stripJavaScriptComments(data)), "\n")
	count := 0
	for index, line := range lines {
		if strings.TrimSpace(line) != "#[test]" || strings.TrimLeft(line, " \t") != line {
			continue
		}
		previous := index - 1
		for previous >= 0 && strings.TrimSpace(lines[previous]) == "" {
			previous--
		}
		if previous >= 0 && strings.HasPrefix(strings.TrimSpace(lines[previous]), "#[") {
			continue
		}
		next := index + 1
		for next < len(lines) && strings.TrimSpace(lines[next]) == "" {
			next++
		}
		if next >= len(lines) || strings.TrimLeft(lines[next], " \t") != lines[next] {
			continue
		}
		declaration := regexp.MustCompile(
			`^(?:async[[:space:]]+)?fn[[:space:]]+` + regexp.QuoteMeta(name) + `[[:space:]]*\(`,
		)
		if declaration.MatchString(strings.TrimSpace(lines[next])) {
			count++
		}
	}
	return count
}

func regexpCaptureContains(pattern *regexp.Regexp, data []byte, value string) bool {
	for _, match := range pattern.FindAllSubmatch(data, -1) {
		if len(match) > 1 && string(match[1]) == value {
			return true
		}
	}
	return false
}

func hasRunnableNodeTest(data []byte, symbol string) bool {
	for _, match := range nodeTestCallPattern.FindAllSubmatch(data, -1) {
		if len(match) != 4 || string(match[3]) != symbol {
			continue
		}
		switch strings.ToLower(string(match[2])) {
		case "", "only":
			return true
		default:
			continue
		}
	}
	return false
}

func stripJavaScriptComments(data []byte) []byte {
	result := append([]byte(nil), data...)
	var quote byte
	escaped := false
	lineComment := false
	blockComment := false
	for index := 0; index < len(result); index++ {
		current := result[index]
		next := byte(0)
		if index+1 < len(result) {
			next = result[index+1]
		}
		switch {
		case lineComment:
			if current == '\n' || current == '\r' {
				lineComment = false
			} else {
				result[index] = ' '
			}
		case blockComment:
			if current == '*' && next == '/' {
				result[index], result[index+1] = ' ', ' '
				index++
				blockComment = false
			} else if current != '\n' && current != '\r' {
				result[index] = ' '
			}
		case quote != 0:
			if escaped {
				escaped = false
			} else if current == '\\' {
				escaped = true
			} else if current == quote {
				quote = 0
			}
		case current == '"' || current == '\'' || current == '`':
			quote = current
		case current == '/' && next == '/':
			result[index], result[index+1] = ' ', ' '
			index++
			lineComment = true
		case current == '/' && next == '*':
			result[index], result[index+1] = ' ', ' '
			index++
			blockComment = true
		}
	}
	return result
}

func productionEscapeEvidenceApplies(evidence ProductionEscapeEvidence, platform string) bool {
	if len(evidence.Platforms) == 0 {
		return true
	}
	for _, candidate := range evidence.Platforms {
		if candidate == platform {
			return true
		}
	}
	return false
}

func productionEscapeEvidenceSourceDigest(repoRoot, relativePath string) (string, error) {
	path := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if !pathWithin(repoRoot, resolved) {
		return "", fmt.Errorf("evidence path resolves outside the repository")
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum), nil
}

func executeProductionEscapeEvidence(repoRoot string, evidence ProductionEscapeEvidence) error {
	path := filepath.Join(repoRoot, filepath.FromSlash(evidence.Path))
	extension := strings.ToLower(filepath.Ext(path))
	leaf := evidence.Symbol
	for _, separator := range []string{"::", ".", "/"} {
		if index := strings.LastIndex(leaf, separator); index >= 0 {
			leaf = leaf[index+len(separator):]
		}
	}

	var command string
	var args []string
	goEvidence := false
	pythonEvidence := false
	nodeEvidence := false
	switch extension {
	case ".go":
		command = "go"
		goEvidence = true
		directory, err := filepath.Rel(repoRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := "."
		if directory != "." {
			pkg = "./" + filepath.ToSlash(directory)
		}
		args = []string{"test", "-json", pkg, "-run", "^" + regexp.QuoteMeta(leaf) + "$", "-count=1"}
	case ".py":
		command = firstAvailableCommand("python3", "python")
		if command == "" {
			return fmt.Errorf("neither python3 nor python is available")
		}
		pythonEvidence = true
		symbol := strings.ReplaceAll(evidence.Symbol, "::", ".")
		symbol = strings.ReplaceAll(symbol, "/", ".")
		args = []string{"-c", pythonProductionEscapeRunner, path, symbol}
	case ".rs":
		manifest, err := findParentFile(repoRoot, filepath.Dir(path), "Cargo.toml")
		if err != nil {
			return err
		}
		command = "cargo"
		target, err := rustIntegrationTarget(manifest, path)
		if err != nil {
			return err
		}
		args = []string{
			"test",
			"--manifest-path", manifest,
			"--test", target,
			evidence.Symbol,
			"--", "--exact",
		}
	case ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts":
		command = firstAvailableCommand("node")
		if command == "" {
			return fmt.Errorf("node is not available")
		}
		reporter, err := os.CreateTemp("", "relkit-node-test-reporter-*.mjs")
		if err != nil {
			return err
		}
		reporterPath := reporter.Name()
		if _, err := reporter.WriteString(nodeProductionEscapeReporter); err != nil {
			reporter.Close()
			os.Remove(reporterPath)
			return err
		}
		if err := reporter.Close(); err != nil {
			os.Remove(reporterPath)
			return err
		}
		defer os.Remove(reporterPath)
		args = []string{
			"--test",
			"--test-name-pattern=^" + regexp.QuoteMeta(evidence.Symbol) + "$",
			"--test-reporter=" + filePathURL(reporterPath),
			path,
		}
		nodeEvidence = true
	default:
		return fmt.Errorf("unsupported test evidence extension %q", extension)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	process := exec.CommandContext(ctx, command, args...)
	process.Dir = repoRoot
	output, err := process.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("%s timed out after 5m", evidence.Path)
	}
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 4000 {
			message = message[len(message)-4000:]
		}
		return fmt.Errorf("%s %s: %w: %s", command, strings.Join(args, " "), err, message)
	}
	if goEvidence {
		if err := validateGoEvidenceExecution(output, leaf); err != nil {
			return err
		}
	}
	if pythonEvidence {
		if err := validatePythonEvidenceExecution(output, evidence.Symbol); err != nil {
			return err
		}
	}
	if extension == ".rs" {
		if err := validateRustEvidenceExecution(output, evidence.Symbol); err != nil {
			return err
		}
	}
	if nodeEvidence {
		if err := validateNodeEvidenceExecution(output, evidence.Symbol, path); err != nil {
			return err
		}
	}
	return nil
}

func validatePythonEvidenceExecution(output []byte, symbol string) error {
	normalizedSymbol := strings.ReplaceAll(symbol, "::", ".")
	normalizedSymbol = strings.ReplaceAll(normalizedSymbol, "/", ".")
	const marker = "RELKIT_PYTHON_TEST_RESULT "
	var result struct {
		Symbol              string `json:"symbol"`
		TestsRun            int    `json:"testsRun"`
		Failures            int    `json:"failures"`
		Errors              int    `json:"errors"`
		Skipped             int    `json:"skipped"`
		ExpectedFailures    int    `json:"expectedFailures"`
		UnexpectedSuccesses int    `json:"unexpectedSuccesses"`
		OK                  bool   `json:"ok"`
	}
	found := false
	for _, rawLine := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, marker) {
			continue
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, marker)), &result); err != nil {
			return fmt.Errorf("decode python test result for %s: %w", symbol, err)
		}
		found = true
	}
	if !found {
		return fmt.Errorf("python unittest did not report a structured result for %s", symbol)
	}
	if result.Symbol != normalizedSymbol || !result.OK || result.TestsRun != 1 ||
		result.Failures != 0 || result.Errors != 0 || result.Skipped != 0 ||
		result.ExpectedFailures != 0 || result.UnexpectedSuccesses != 0 {
		return fmt.Errorf("python unittest did not pass exactly one unskipped %s", symbol)
	}
	return nil
}

func validateNodeEvidenceExecution(output []byte, symbol, expectedPath string) error {
	const marker = "RELKIT_NODE_TEST_RESULT "
	matches := 0
	for _, rawLine := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, marker) {
			continue
		}
		var event struct {
			Type string `json:"type"`
			Name string `json:"name"`
			Kind string `json:"kind"`
			File string `json:"file"`
			Skip bool   `json:"skip"`
			Todo bool   `json:"todo"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, marker)), &event); err != nil {
			return fmt.Errorf("decode node test result for %s: %w", symbol, err)
		}
		if event.Name != symbol {
			continue
		}
		if !sameFilePath(event.File, expectedPath) {
			return fmt.Errorf(
				"node test %s ran from %q, expected evidence file %q",
				symbol,
				event.File,
				expectedPath,
			)
		}
		if event.Type != "test:pass" || event.Kind != "test" || event.Skip || event.Todo {
			return fmt.Errorf("node test did not pass %s", symbol)
		}
		matches++
	}
	if matches != 1 {
		return fmt.Errorf("node test reported %d passing matches for %s, expected exactly one", matches, symbol)
	}
	return nil
}

func sameFilePath(value, expected string) bool {
	if strings.HasPrefix(value, "file://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return false
		}
		value = parsed.Path
		if runtime.GOOS == "windows" && len(value) >= 3 && value[0] == '/' && value[2] == ':' {
			value = value[1:]
		}
	}
	normalize := func(path string) string {
		absolute, err := filepath.Abs(path)
		if err == nil {
			path = absolute
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		return filepath.Clean(path)
	}
	value = normalize(value)
	expected = normalize(expected)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(value, expected)
	}
	return value == expected
}

func filePathURL(path string) string {
	absolute, err := filepath.Abs(path)
	if err == nil {
		path = absolute
	}
	slashPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	return (&url.URL{Scheme: "file", Path: slashPath}).String()
}

func validateRustEvidenceExecution(output []byte, symbol string) error {
	normalizedSymbol := strings.ReplaceAll(symbol, ".", "::")
	normalizedSymbol = strings.ReplaceAll(normalizedSymbol, "/", "::")
	passedLine := "test " + normalizedSymbol + " ... ok"
	statusPrefix := "test " + normalizedSymbol + " ... "
	passed := 0
	statuses := 0
	for _, rawLine := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, statusPrefix) {
			statuses++
		}
		if line == passedLine {
			passed++
		}
	}
	if statuses != 1 || passed != 1 {
		return fmt.Errorf(
			"cargo test reported %d statuses and %d exact passes for %s, expected one pass only",
			statuses,
			passed,
			symbol,
		)
	}
	return nil
}

func validateGoEvidenceExecution(output []byte, symbol string) error {
	decoder := json.NewDecoder(bytes.NewReader(output))
	ran := false
	passed := false
	skipped := false
	for {
		var event struct {
			Action string `json:"Action"`
			Test   string `json:"Test"`
		}
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("decode go test event for %s: %w", symbol, err)
		}
		if event.Test != symbol {
			continue
		}
		switch event.Action {
		case "run":
			ran = true
		case "pass":
			passed = true
		case "skip":
			skipped = true
		}
	}
	switch {
	case skipped:
		return fmt.Errorf("go test skipped %s on %s", symbol, runtime.GOOS)
	case !ran:
		return fmt.Errorf("go test did not run %s", symbol)
	case !passed:
		return fmt.Errorf("go test did not report %s as passed", symbol)
	default:
		return nil
	}
}

func firstAvailableCommand(names ...string) string {
	for _, name := range names {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return ""
}

func findParentFile(repoRoot, start, name string) (string, error) {
	current := filepath.Clean(start)
	for pathWithin(repoRoot, current) {
		candidate := filepath.Join(current, name)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
		if current == repoRoot {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", fmt.Errorf("%s has no %s within the repository", start, name)
}

func productionEscapeGitSourceState(repoRoot string) (string, bool, error) {
	commitOutput, err := gitOutput(repoRoot, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", false, err
	}
	commit := strings.TrimSpace(string(commitOutput))
	if !escapeCommitPattern.MatchString(commit) {
		return "", false, fmt.Errorf("HEAD did not resolve to a full Git commit")
	}
	statusOutput, err := gitOutput(repoRoot, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return "", false, err
	}
	return commit, strings.TrimSpace(string(statusOutput)) != "", nil
}

func verifyProductionEscapeSourcesAtHEAD(
	repoRoot string,
	registry string,
	recordFiles []string,
	records map[string]ProductionEscape,
) []ProductionEscapeAuditFinding {
	findings := []ProductionEscapeAuditFinding{}
	registryRelative, err := filepath.Rel(repoRoot, registry)
	if err != nil {
		return append(findings, ProductionEscapeAuditFinding{
			Code: "production_escape.source_invalid", Path: filepath.ToSlash(registry),
			Message: err.Error(),
		})
	}
	pathCodes := map[string]string{}
	for _, name := range recordFiles {
		pathCodes[filepath.ToSlash(filepath.Join(registryRelative, name))] = ""
	}
	for code, record := range records {
		for _, evidence := range record.Evidence {
			if _, exists := pathCodes[evidence.Path]; !exists {
				pathCodes[evidence.Path] = code
			}
		}
	}
	paths := make([]string, 0, len(pathCodes))
	for path := range pathCodes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if _, err := gitOutput(repoRoot, "ls-files", "--error-unmatch", "--", path); err != nil {
			findings = append(findings, ProductionEscapeAuditFinding{
				Code:       "production_escape.source_untracked",
				Path:       path,
				EscapeCode: pathCodes[path],
				Message:    "registry and evidence inputs must be tracked by HEAD even when Git ignore rules hide them",
			})
			continue
		}
		headData, err := gitOutput(repoRoot, "show", "HEAD:"+path)
		if err != nil {
			findings = append(findings, ProductionEscapeAuditFinding{
				Code:       "production_escape.source_invalid",
				Path:       path,
				EscapeCode: pathCodes[path],
				Message:    err.Error(),
			})
			continue
		}
		workingData, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(path)))
		if err != nil {
			findings = append(findings, ProductionEscapeAuditFinding{
				Code:       "production_escape.source_invalid",
				Path:       path,
				EscapeCode: pathCodes[path],
				Message:    err.Error(),
			})
			continue
		}
		normalize := func(value []byte) []byte {
			return bytes.ReplaceAll(value, []byte("\r\n"), []byte("\n"))
		}
		if !bytes.Equal(normalize(headData), normalize(workingData)) {
			findings = append(findings, ProductionEscapeAuditFinding{
				Code:       "production_escape.source_mismatch",
				Path:       path,
				EscapeCode: pathCodes[path],
				Message:    "working-tree content does not match the blob recorded at HEAD",
			})
		}
	}
	return findings
}

func auditProductionEscapeBaseline(
	repoRoot string,
	registry string,
	baselineRef string,
	current map[string]ProductionEscape,
) []ProductionEscapeAuditFinding {
	findings := []ProductionEscapeAuditFinding{}
	add := func(code, path, escapeCode, message string) {
		findings = append(findings, ProductionEscapeAuditFinding{
			Code: code, Path: filepath.ToSlash(path), EscapeCode: escapeCode, Message: message,
		})
	}
	registryRelative, err := filepath.Rel(repoRoot, registry)
	if err != nil || !pathWithin(repoRoot, registry) {
		add("production_escape.baseline_invalid", baselineRef, "", "cannot derive repo-relative registry path")
		return findings
	}
	registryRelative = filepath.ToSlash(registryRelative)
	output, err := gitOutput(repoRoot, "ls-tree", "-r", "--name-only", baselineRef, "--", registryRelative)
	if err != nil {
		add("production_escape.baseline_invalid", baselineRef, "", err.Error())
		return findings
	}
	baseline := map[string]ProductionEscape{}
	baselinePath := map[string]string{}
	for _, path := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		path = strings.TrimSpace(path)
		if path == "" || filepath.Ext(path) != ".json" ||
			filepath.ToSlash(filepath.Dir(path)) != registryRelative {
			continue
		}
		data, showErr := gitOutput(repoRoot, "show", baselineRef+":"+path)
		if showErr != nil {
			add("production_escape.baseline_invalid", path, "", showErr.Error())
			continue
		}
		var record ProductionEscape
		if decodeErr := decodeProductionEscape(data, &record); decodeErr != nil {
			add("production_escape.baseline_invalid", path, record.Code, decodeErr.Error())
			continue
		}
		if previous, duplicate := baselinePath[record.Code]; duplicate {
			add(
				"production_escape.baseline_invalid",
				path,
				record.Code,
				fmt.Sprintf("baseline code duplicates %s", previous),
			)
			continue
		}
		baseline[record.Code] = record
		baselinePath[record.Code] = path
	}
	for code, previous := range baseline {
		record, ok := current[code]
		if !ok {
			add(
				"production_escape.record_removed",
				baselinePath[code],
				code,
				fmt.Sprintf("record present in baseline %s must not be deleted or renamed", baselineRef),
			)
			continue
		}
		if record.Schema != previous.Schema ||
			record.Category != previous.Category ||
			record.Stage != previous.Stage ||
			record.Capability != previous.Capability ||
			!equalOptionalBool(record.CrossComponent, previous.CrossComponent) {
			add(
				"production_escape.identity_changed",
				baselinePath[code],
				code,
				"schema, category, stage, capability, and crossComponent are immutable once merged",
			)
		}
		if fixFieldChanged(previous.Fix, record.Fix) {
			add(
				"production_escape.closure_regressed",
				baselinePath[code],
				code,
				"non-empty fix.pr, fix.commit, and fix.release values are immutable once merged",
			)
		}
	}
	return findings
}

func decodeProductionEscape(data []byte, target *ProductionEscape) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("document contains more than one JSON value")
		}
		return err
	}
	return nil
}

func equalOptionalBool(left, right *bool) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func fixFieldChanged(previous, current *ProductionEscapeFix) bool {
	if previous == nil {
		return false
	}
	if previous.PR != nil {
		if current == nil || current.PR == nil || *current.PR != *previous.PR {
			return true
		}
	}
	if previous.Commit != "" && (current == nil || current.Commit != previous.Commit) {
		return true
	}
	return previous.Release != "" && (current == nil || current.Release != previous.Release)
}

func verifyProductionEscapeFixHistory(repoRoot string, fix ProductionEscapeFix, auditedRelease string) error {
	if err := requireGitCommit(repoRoot, fix.Commit); err != nil {
		return fmt.Errorf("fix.commit %s is not available as a Git commit: %w", fix.Commit, err)
	}
	releaseRef := "refs/tags/" + fix.Release
	if err := requireGitCommit(repoRoot, releaseRef); err != nil {
		return fmt.Errorf("fix.release tag %s is not available as a Git commit: %w", fix.Release, err)
	}
	if err := requireGitAncestor(repoRoot, fix.Commit, releaseRef); err != nil {
		return fmt.Errorf("fix.commit %s is not contained in fix.release %s: %w", fix.Commit, fix.Release, err)
	}
	auditedRef := "refs/tags/" + auditedRelease
	if err := requireGitAncestor(repoRoot, releaseRef, auditedRef); err != nil {
		return fmt.Errorf("fix.release %s is not contained in audited release %s: %w", fix.Release, auditedRelease, err)
	}
	return nil
}

func requireGitCommit(repoRoot, reference string) error {
	_, err := resolveGitCommit(repoRoot, reference)
	return err
}

func resolveGitCommit(repoRoot, reference string) (string, error) {
	output, err := gitOutput(repoRoot, "rev-parse", "--verify", reference+"^{commit}")
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(string(output))
	if !escapeCommitPattern.MatchString(commit) {
		return "", fmt.Errorf("%s did not resolve to a full Git commit", reference)
	}
	return commit, nil
}

func requireGitAncestor(repoRoot, ancestor, descendant string) error {
	command := exec.Command("git", "-C", repoRoot, "merge-base", "--is-ancestor", ancestor, descendant)
	if output, err := command.CombinedOutput(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			return fmt.Errorf("%s is not an ancestor of %s", ancestor, descendant)
		}
		return fmt.Errorf("git merge-base --is-ancestor: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func gitOutput(repoRoot string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", repoRoot}, args...)
	command := exec.Command("git", commandArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func isPortableWindowsAbsolute(path string) bool {
	if len(path) < 3 || path[1] != ':' || path[2] != '/' {
		return false
	}
	first := path[0]
	return first >= 'A' && first <= 'Z' || first >= 'a' && first <= 'z'
}

func canonicalDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("repo root must not be empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	return filepath.Clean(resolved), nil
}

func resolveRegistryDirectory(repoRoot, registry string) (string, error) {
	if strings.TrimSpace(registry) == "" {
		return "", fmt.Errorf("registry path must not be empty")
	}
	path := registry
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	if !pathWithin(repoRoot, resolved) {
		return "", fmt.Errorf("registry path resolves outside the repository")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("registry path is not a directory")
	}
	return filepath.Clean(resolved), nil
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func finishProductionEscapeAudit(report *ProductionEscapeAuditReport) (*ProductionEscapeAuditReport, error) {
	report.OK = len(report.Findings) == 0
	if report.OK {
		return report, nil
	}
	return report, ProductionEscapeAuditError{Findings: len(report.Findings)}
}
