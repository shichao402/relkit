package ops

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/extension"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const (
	FleetSchema               = "relkit.ops-fleet/1"
	ImpactSchema              = "relkit.impact/1"
	CertificationInputSchema  = "relkit.ops-certification-input/1"
	CertificationReportSchema = "relkit.certification-report/1"
)

type Risk string

const (
	RiskDocumentation    Risk = "documentation"
	RiskSingleCapability Risk = "single-capability"
	RiskContract         Risk = "contract"
	RiskCrossCutting     Risk = "cross-cutting"
)

type Impact struct {
	Schema                   string                 `json:"schema"`
	CandidateDigest          string                 `json:"candidateDigest"`
	Risk                     Risk                   `json:"risk"`
	AffectedCapabilities     []contracts.Capability `json:"affectedCapabilities,omitempty"`
	AffectedComponents       []string               `json:"affectedComponents,omitempty"`
	NewCapabilities          []contracts.Capability `json:"newCapabilities,omitempty"`
	CandidateProfilePath     string                 `json:"candidateProfilePath"`
	CandidateCatalogPath     string                 `json:"candidateCatalogPath"`
	CandidateRootFingerprint string                 `json:"candidateRootFingerprint"`
}

type GenerateImpactOptions struct {
	ProductionCatalogPath    string
	CandidateCatalogPath     string
	ProductionTrustKeyPaths  []string
	CandidateTrustKeyPaths   []string
	CandidateDigest          string
	CandidateProfilePath     string
	CandidateRootFingerprint string
	Risk                     Risk
}

func GenerateImpact(options GenerateImpactOptions) (*Impact, error) {
	productionCatalog, err := catalog.Load(options.ProductionCatalogPath)
	if err != nil {
		return nil, err
	}
	productionRoots, err := loadTrustKeys(options.ProductionTrustKeyPaths)
	if err != nil {
		return nil, err
	}
	if err := productionCatalog.Verify(productionRoots); err != nil {
		return nil, fmt.Errorf("production catalog: %w", err)
	}
	candidateCatalog, err := catalog.Load(options.CandidateCatalogPath)
	if err != nil {
		return nil, err
	}
	candidateRoots, err := loadTrustKeys(options.CandidateTrustKeyPaths)
	if err != nil {
		return nil, err
	}
	matchingCandidateRoots, err := matchingKeys(options.CandidateRootFingerprint, candidateRoots)
	if err != nil {
		return nil, err
	}
	if err := candidateCatalog.Verify(matchingCandidateRoots); err != nil {
		return nil, fmt.Errorf("candidate catalog: %w", err)
	}
	candidateProfile, err := operator.Load(options.CandidateProfilePath)
	if err != nil {
		return nil, err
	}
	if err := candidateProfile.Verify(matchingCandidateRoots); err != nil {
		return nil, fmt.Errorf("candidate Operator Profile: %w", err)
	}
	candidate, err := findDistribution(*candidateCatalog, options.CandidateDigest)
	if err != nil {
		return nil, err
	}
	if candidate.State != catalog.StateCandidate {
		return nil, fmt.Errorf("distribution %s is not in candidate state", candidate.Digest)
	}
	if candidateProfile.ServerRelease.DistributionDigest != candidate.Digest {
		return nil, fmt.Errorf("candidate Operator Profile selects %s, impact target is %s", candidateProfile.ServerRelease.DistributionDigest, candidate.Digest)
	}
	production, err := productionCatalog.Production()
	if err != nil {
		return nil, err
	}
	if len(production) == 0 {
		return nil, fmt.Errorf("production catalog has no certified baseline")
	}
	baseline := production[0]
	affectedComponents := changedComponents(baseline, candidate)
	affectedCapabilities, newCapabilities := changedCapabilities(baseline, candidate)
	impact := &Impact{
		Schema: ImpactSchema, CandidateDigest: candidate.Digest, Risk: options.Risk,
		AffectedCapabilities: affectedCapabilities, AffectedComponents: affectedComponents,
		NewCapabilities: newCapabilities, CandidateProfilePath: options.CandidateProfilePath,
		CandidateCatalogPath:     options.CandidateCatalogPath,
		CandidateRootFingerprint: options.CandidateRootFingerprint,
	}
	if err := impact.Validate(); err != nil {
		return nil, err
	}
	return impact, nil
}

func LoadImpact(path string) (*Impact, error) {
	var value Impact
	if err := document.Load(path, &value); err != nil {
		return nil, err
	}
	base := filepath.Dir(path)
	if value.CandidateProfilePath != "" && !filepath.IsAbs(value.CandidateProfilePath) {
		value.CandidateProfilePath = filepath.Join(base, value.CandidateProfilePath)
	}
	if value.CandidateCatalogPath != "" && !filepath.IsAbs(value.CandidateCatalogPath) {
		value.CandidateCatalogPath = filepath.Join(base, value.CandidateCatalogPath)
	}
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &value, nil
}

func (i Impact) Validate() error {
	if i.Schema != ImpactSchema {
		return fmt.Errorf("schema is %q, expected %q", i.Schema, ImpactSchema)
	}
	if err := canonical.ValidateDigest(i.CandidateDigest); err != nil {
		return fmt.Errorf("candidateDigest: %w", err)
	}
	switch i.Risk {
	case RiskDocumentation, RiskSingleCapability, RiskContract, RiskCrossCutting:
	default:
		return fmt.Errorf("unknown risk %q", i.Risk)
	}
	if err := (contracts.Vector{Requires: i.AffectedCapabilities, Provides: i.NewCapabilities}).Validate(); err != nil {
		return err
	}
	if i.Risk != RiskDocumentation {
		if i.CandidateProfilePath == "" || i.CandidateCatalogPath == "" {
			return fmt.Errorf("candidate Profile and Catalog paths are required")
		}
		if err := canonical.ValidateDigest(i.CandidateRootFingerprint); err != nil {
			return fmt.Errorf("candidateRootFingerprint: %w", err)
		}
	}
	return nil
}

type CanaryKind string

const (
	CanaryNone     CanaryKind = "none"
	CanaryFixed    CanaryKind = "fixed"
	CanaryRotating CanaryKind = "rotating"
)

type FleetHost struct {
	ID                    string                    `json:"id"`
	Active                bool                      `json:"active"`
	ProjectPath           string                    `json:"projectPath"`
	Canary                CanaryKind                `json:"canary"`
	MandatoryCapabilities []contracts.Capability    `json:"mandatoryCapabilities,omitempty"`
	RequiredCI            []string                  `json:"requiredCi"`
	CIAdapters            map[string]extension.Spec `json:"ciAdapters"`
}

type Fleet struct {
	Schema string      `json:"schema"`
	Hosts  []FleetHost `json:"hosts"`
}

func LoadFleet(path string) (*Fleet, error) {
	var value Fleet
	if err := document.Load(path, &value); err != nil {
		return nil, err
	}
	base := filepath.Dir(path)
	for index := range value.Hosts {
		host := &value.Hosts[index]
		if !filepath.IsAbs(host.ProjectPath) {
			host.ProjectPath = filepath.Join(base, host.ProjectPath)
		}
		for name, adapter := range host.CIAdapters {
			if adapter.WorkingDirectory != "" && !filepath.IsAbs(adapter.WorkingDirectory) {
				adapter.WorkingDirectory = filepath.Join(base, adapter.WorkingDirectory)
			}
			host.CIAdapters[name] = adapter
		}
	}
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &value, nil
}

func (f Fleet) Validate() error {
	if f.Schema != FleetSchema {
		return fmt.Errorf("schema is %q, expected %q", f.Schema, FleetSchema)
	}
	seen := map[string]bool{}
	for index, host := range f.Hosts {
		if host.ID == "" || seen[host.ID] || host.ProjectPath == "" {
			return fmt.Errorf("hosts[%d] has empty/duplicate id or missing projectPath", index)
		}
		seen[host.ID] = true
		if host.Active && len(host.RequiredCI) == 0 {
			return fmt.Errorf("active host %s has no required CI path", host.ID)
		}
		switch host.Canary {
		case CanaryNone, CanaryFixed, CanaryRotating:
		default:
			return fmt.Errorf("host %s has invalid canary kind %q", host.ID, host.Canary)
		}
		if err := (contracts.Vector{Requires: host.MandatoryCapabilities}).Validate(); err != nil {
			return fmt.Errorf("host %s mandatoryCapabilities: %w", host.ID, err)
		}
		required := map[string]bool{}
		for _, name := range host.RequiredCI {
			if name == "" || required[name] {
				return fmt.Errorf("host %s requiredCi contains an empty or duplicate name", host.ID)
			}
			required[name] = true
			adapter, ok := host.CIAdapters[name]
			if !ok {
				continue // Reported as skipped-unverified at execution time.
			}
			if err := validateAdapter(name, adapter, extension.KindCIAdapter, CertificationInputSchema); err != nil {
				return fmt.Errorf("host %s CI adapter %s: %w", host.ID, name, err)
			}
		}
	}
	return nil
}

type SelectedHost struct {
	Host          FleetHost
	Project       project.Document
	ProjectDigest string
	Cohort        string
	Reason        string
}

func SelectFleet(fleet Fleet, impact Impact, full bool) ([]SelectedHost, error) {
	if err := fleet.Validate(); err != nil {
		return nil, err
	}
	if err := impact.Validate(); err != nil {
		return nil, err
	}
	var eligible []SelectedHost
	for _, host := range fleet.Hosts {
		if !host.Active {
			continue
		}
		product, err := project.Load(host.ProjectPath)
		if err != nil {
			return nil, fmt.Errorf("fleet host %s: %w", host.ID, err)
		}
		digest, err := product.Digest()
		if err != nil {
			return nil, err
		}
		cohort, err := canonical.Digest(product.CapabilityFingerprint)
		if err != nil {
			return nil, err
		}
		reason := impactReason(*product, host, impact, full)
		if reason == "" {
			continue
		}
		eligible = append(eligible, SelectedHost{
			Host: host, Project: *product, ProjectDigest: digest, Cohort: cohort, Reason: reason,
		})
	}
	if full || impact.Risk == RiskCrossCutting {
		sortSelected(eligible)
		return eligible, nil
	}
	if impact.Risk == RiskDocumentation {
		return nil, nil
	}

	// Keep every fixed and mandatory canary. Add one deterministic rotating
	// sample per affected capability cohort.
	selected := map[string]SelectedHost{}
	rotating := map[string][]SelectedHost{}
	for _, item := range eligible {
		switch {
		case strings.HasPrefix(item.Reason, "new-capability:"), item.Host.Canary == CanaryFixed:
			selected[item.Host.ID] = item
		case item.Host.Canary == CanaryRotating:
			rotating[item.Cohort] = append(rotating[item.Cohort], item)
		}
	}
	for _, cohort := range rotating {
		sort.Slice(cohort, func(left, right int) bool { return cohort[left].Host.ID < cohort[right].Host.ID })
		selected[cohort[0].Host.ID] = cohort[0]
	}
	result := make([]SelectedHost, 0, len(selected))
	for _, item := range selected {
		result = append(result, item)
	}
	sortSelected(result)
	return result, nil
}

func impactReason(product project.Document, host FleetHost, impact Impact, full bool) string {
	if full || impact.Risk == RiskCrossCutting {
		return "full"
	}
	if impact.Risk == RiskDocumentation {
		return ""
	}
	for _, newCapability := range impact.NewCapabilities {
		for _, mandatory := range host.MandatoryCapabilities {
			if contracts.Compatible(newCapability, mandatory) || contracts.Compatible(mandatory, newCapability) {
				return "new-capability:" + string(newCapability.Domain) + "/" + newCapability.Peer
			}
		}
	}
	for _, affected := range impact.AffectedCapabilities {
		for _, required := range product.CapabilityFingerprint.RequiredContracts {
			if contracts.Compatible(affected, required) || contracts.Compatible(required, affected) {
				return "affected-contract:" + string(affected.Domain) + "/" + affected.Peer
			}
		}
	}
	for _, component := range impact.AffectedComponents {
		if componentUsed(product.CapabilityFingerprint, component) {
			return "affected-component:" + component
		}
	}
	return ""
}

func componentUsed(fingerprint project.CapabilityFingerprint, component string) bool {
	switch component {
	case "cli":
		return true
	case "updater":
		return len(fingerprint.Facades) > 0
	case "sdk-go", "sdk-rust", "sdk-dart", "sdk-node":
		return containsString(fingerprint.Facades, strings.TrimPrefix(component, "sdk-"))
	case "bindings-ts":
		return containsString(fingerprint.Projections, "ts") || containsString(fingerprint.Projections, "typescript")
	default:
		return false
	}
}

type CertificationInput struct {
	Schema                   string `json:"schema"`
	CandidateDigest          string `json:"candidateDigest"`
	CandidateProfilePath     string `json:"candidateProfilePath"`
	CandidateCatalogPath     string `json:"candidateCatalogPath"`
	CandidateRootFingerprint string `json:"candidateRootFingerprint"`
	HostID                   string `json:"hostId"`
	ProjectPath              string `json:"projectPath"`
	ProjectDigest            string `json:"projectDigest"`
	Cohort                   string `json:"cohort"`
	Reason                   string `json:"reason"`
}

type CertificationResult struct {
	HostID         string         `json:"hostId"`
	CI             string         `json:"ci"`
	Status         string         `json:"status"`
	DiagnosticCode string         `json:"diagnosticCode,omitempty"`
	Evidence       map[string]any `json:"evidence,omitempty"`
	Message        string         `json:"message,omitempty"`
}

type CertificationReport struct {
	Schema          string                `json:"schema"`
	CandidateDigest string                `json:"candidateDigest"`
	ImpactDigest    string                `json:"impactDigest"`
	Full            bool                  `json:"full"`
	Passed          bool                  `json:"passed"`
	StartedAt       string                `json:"startedAt"`
	FinishedAt      string                `json:"finishedAt"`
	SelectedHosts   []string              `json:"selectedHosts"`
	Results         []CertificationResult `json:"results"`
}

func RunCertification(ctx context.Context, fleet Fleet, impact Impact, full bool) (*CertificationReport, error) {
	selected, err := SelectFleet(fleet, impact, full)
	if err != nil {
		return nil, err
	}
	impactDigest, err := canonical.Digest(impact)
	if err != nil {
		return nil, err
	}
	report := &CertificationReport{
		Schema: CertificationReportSchema, CandidateDigest: impact.CandidateDigest,
		ImpactDigest: impactDigest, Full: full, Passed: true,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	for _, item := range selected {
		report.SelectedHosts = append(report.SelectedHosts, item.Host.ID)
		for _, name := range item.Host.RequiredCI {
			adapter, ok := item.Host.CIAdapters[name]
			if !ok {
				report.Passed = false
				report.Results = append(report.Results, CertificationResult{
					HostID: item.Host.ID, CI: name, Status: "skipped-unverified",
					DiagnosticCode: "certification.ci_adapter_missing",
					Message:        "required CI path has no adapter",
				})
				continue
			}
			var output StepResult
			runErr := extension.Run(ctx, adapter, CertificationInput{
				Schema: CertificationInputSchema, CandidateDigest: impact.CandidateDigest,
				CandidateProfilePath:     impact.CandidateProfilePath,
				CandidateCatalogPath:     impact.CandidateCatalogPath,
				CandidateRootFingerprint: impact.CandidateRootFingerprint,
				HostID:                   item.Host.ID, ProjectPath: item.Host.ProjectPath,
				ProjectDigest: item.ProjectDigest, Cohort: item.Cohort, Reason: item.Reason,
			}, &output)
			result := CertificationResult{HostID: item.Host.ID, CI: name}
			switch {
			case runErr != nil:
				report.Passed = false
				result.Status = "failed"
				result.DiagnosticCode = "certification.adapter_failed"
				result.Message = runErr.Error()
			case output.Schema != StepResultSchema:
				report.Passed = false
				result.Status = "failed"
				result.DiagnosticCode = "certification.output_invalid"
				result.Message = fmt.Sprintf("adapter returned schema %q", output.Schema)
			case !output.OK:
				report.Passed = false
				result.Status = "failed"
				result.DiagnosticCode = evidenceCode(output.Evidence)
				result.Evidence = output.Evidence
			default:
				result.Status = "passed"
				result.Evidence = output.Evidence
			}
			report.Results = append(report.Results, result)
		}
	}
	if impact.Risk != RiskDocumentation && len(selected) == 0 {
		report.Passed = false
		report.Results = append(report.Results, CertificationResult{
			Status: "skipped-unverified", DiagnosticCode: "certification.no_canary",
			Message: "impact selected no active canary",
		})
	}
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return report, nil
}

func (r CertificationReport) ValidateFor(candidateDigest string) error {
	if r.Schema != CertificationReportSchema || r.CandidateDigest != candidateDigest {
		return fmt.Errorf("certification report does not match candidate %s", candidateDigest)
	}
	if !r.Passed {
		return fmt.Errorf("candidate certification did not pass")
	}
	if len(r.Results) == 0 {
		return fmt.Errorf("certification report has no verified result")
	}
	for _, result := range r.Results {
		if result.Status != "passed" {
			return fmt.Errorf("certification result %s/%s is %s", result.HostID, result.CI, result.Status)
		}
	}
	return nil
}

func evidenceCode(evidence map[string]any) string {
	if value, ok := evidence["diagnosticCode"].(string); ok && value != "" {
		return value
	}
	return "certification.reported_failure"
}

func sortSelected(values []SelectedHost) {
	sort.Slice(values, func(left, right int) bool { return values[left].Host.ID < values[right].Host.ID })
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func matchingKeys(fingerprint string, values map[string]ed25519.PublicKey) (map[string]ed25519.PublicKey, error) {
	if err := canonical.ValidateDigest(fingerprint); err != nil {
		return nil, fmt.Errorf("Candidate Root fingerprint: %w", err)
	}
	result := map[string]ed25519.PublicKey{}
	for keyID, key := range values {
		value, err := trust.Fingerprint(key)
		if err != nil {
			return nil, err
		}
		if value == fingerprint {
			result[keyID] = key
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no Candidate Root key matches fingerprint %s", fingerprint)
	}
	return result, nil
}

func changedComponents(baseline, candidate catalog.Distribution) []string {
	left := map[string]catalog.Component{}
	right := map[string]catalog.Component{}
	for _, component := range baseline.Components {
		left[component.Name+"\x00"+component.Platform] = component
	}
	for _, component := range candidate.Components {
		right[component.Name+"\x00"+component.Platform] = component
	}
	names := map[string]bool{}
	for key, component := range left {
		other, ok := right[key]
		if !ok || !canonicalEqual(component, other) {
			names[component.Name] = true
		}
	}
	for key, component := range right {
		other, ok := left[key]
		if !ok || !canonicalEqual(component, other) {
			names[component.Name] = true
		}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func changedCapabilities(baseline, candidate catalog.Distribution) ([]contracts.Capability, []contracts.Capability) {
	left := capabilityMap(append(append([]contracts.Capability{}, baseline.Requires...), baseline.Provides...))
	right := capabilityMap(append(append([]contracts.Capability{}, candidate.Requires...), candidate.Provides...))
	var affected []contracts.Capability
	var added []contracts.Capability
	for key, capability := range left {
		other, ok := right[key]
		if !ok {
			affected = append(affected, capability)
		} else if !canonicalEqual(capability, other) {
			affected = append(affected, other)
		}
	}
	for key, capability := range right {
		if _, ok := left[key]; !ok {
			affected = append(affected, capability)
			added = append(added, capability)
		}
	}
	sortCapabilities(affected)
	sortCapabilities(added)
	return affected, added
}

func capabilityMap(values []contracts.Capability) map[string]contracts.Capability {
	result := make(map[string]contracts.Capability, len(values))
	for _, value := range values {
		result[string(value.Domain)+"\x00"+value.Peer] = value
	}
	return result
}

func sortCapabilities(values []contracts.Capability) {
	sort.Slice(values, func(left, right int) bool {
		l := string(values[left].Domain) + "\x00" + values[left].Peer
		r := string(values[right].Domain) + "\x00" + values[right].Peer
		return l < r
	})
}

func canonicalEqual(left, right any) bool {
	leftBytes, leftErr := canonical.Bytes(left)
	rightBytes, rightErr := canonical.Bytes(right)
	return leftErr == nil && rightErr == nil && string(leftBytes) == string(rightBytes)
}
