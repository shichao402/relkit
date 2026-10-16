package ops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/diag"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/extension"
	"github.com/shichao402/relkit/internal/vnext/operator"
)

type UpgradePlan struct {
	Schema               string               `json:"schema"`
	OperationID          string               `json:"operationId"`
	Operator             string               `json:"operator"`
	CurrentProfileDigest string               `json:"currentProfileDigest"`
	CurrentSequence      int64                `json:"currentSequence"`
	CatalogDigest        string               `json:"catalogDigest"`
	CatalogSequence      int64                `json:"catalogSequence"`
	Target               catalog.Distribution `json:"target"`
	Services             []string             `json:"services,omitempty"`
	Mirrors              []string             `json:"mirrors,omitempty"`
	ContractChanges      map[string]any       `json:"contractChanges"`
	RequiredPhases       []string             `json:"requiredPhases"`
}

func (p UpgradePlan) Validate() error {
	if p.Schema != UpgradePlanSchema {
		return fmt.Errorf("schema is %q, expected %q", p.Schema, UpgradePlanSchema)
	}
	if err := validateOperationID(p.OperationID); err != nil {
		return err
	}
	if p.Operator == "" || p.CurrentProfileDigest == "" || p.CatalogDigest == "" {
		return fmt.Errorf("operator and control-plane digests are required")
	}
	if p.Target.State != catalog.StateCertified {
		return fmt.Errorf("target distribution is not certified")
	}
	return nil
}

func PlanUpgrade(inventory Inventory, to string) (*UpgradePlan, error) {
	profile, distributions, _, err := loadTrustedState(inventory)
	if err != nil {
		return nil, operationError("operator.control_plane_invalid", "upgrade-plan", "Operator control plane is not trusted", nil, err)
	}
	target, err := selectUpgradeTarget(*distributions, to)
	if err != nil {
		return nil, operationError("operator.target_unavailable", "upgrade-plan", "No certified upgrade target satisfies the request", map[string]any{"to": to}, err)
	}
	profileDigest, err := profile.Digest()
	if err != nil {
		return nil, err
	}
	catalogDigest, err := distributions.Digest()
	if err != nil {
		return nil, err
	}
	operationID, err := newOperationID("upgrade")
	if err != nil {
		return nil, err
	}
	plan := &UpgradePlan{
		Schema:               UpgradePlanSchema,
		OperationID:          operationID,
		Operator:             inventory.Operator,
		CurrentProfileDigest: profileDigest,
		CurrentSequence:      profile.Sequence,
		CatalogDigest:        catalogDigest,
		CatalogSequence:      distributions.Sequence,
		Target:               target,
		Services:             append([]string(nil), inventory.Services...),
		Mirrors:              append([]string(nil), inventory.Mirrors...),
		ContractChanges: map[string]any{
			"from": profile.Provides,
			"to":   target.Provides,
		},
		RequiredPhases: []string{"candidate", "sync", "deploy", "verify", "activate"},
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	return plan, nil
}

func WriteUpgradePlan(path string, plan UpgradePlan) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	return document.WriteNew(path, plan)
}

type UpgradeStepInput struct {
	Schema             string               `json:"schema"`
	OperationID        string               `json:"operationId"`
	Phase              string               `json:"phase"`
	Operator           string               `json:"operator"`
	Mirrors            []string             `json:"mirrors,omitempty"`
	CurrentProfile     operator.Document    `json:"currentProfile"`
	TargetDistribution catalog.Distribution `json:"targetDistribution"`
	TargetProfilePath  string               `json:"targetProfilePath,omitempty"`
	PriorEvidence      map[string]any       `json:"priorEvidence,omitempty"`
}

type OperationState struct {
	Schema      string                    `json:"schema"`
	OperationID string                    `json:"operationId"`
	Kind        string                    `json:"kind"`
	Operator    string                    `json:"operator"`
	Status      string                    `json:"status"`
	Phase       string                    `json:"phase"`
	StartedAt   string                    `json:"startedAt"`
	FinishedAt  string                    `json:"finishedAt,omitempty"`
	Evidence    map[string]map[string]any `json:"evidence,omitempty"`
	Diagnostic  *diag.Envelope            `json:"diagnostic,omitempty"`
}

func ApplyUpgrade(ctx context.Context, inventory Inventory, plan UpgradePlan) (*operator.Document, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	profile, distributions, _, err := loadTrustedState(inventory)
	if err != nil {
		return nil, operationError("operator.control_plane_invalid", "upgrade", "Operator control plane is not trusted", nil, err)
	}
	if plan.Operator != inventory.Operator {
		return nil, operationError("operator.plan_wrong_target", "upgrade", "Upgrade plan targets another Operator", nil, nil)
	}
	if !slices.Equal(plan.Services, inventory.Services) || !slices.Equal(plan.Mirrors, inventory.Mirrors) {
		return nil, operationError(
			"operator.plan_stale",
			"upgrade",
			"Operator services or mirrors changed after the plan was created",
			map[string]any{
				"plannedServices": plan.Services,
				"currentServices": inventory.Services,
				"plannedMirrors":  plan.Mirrors,
				"currentMirrors":  inventory.Mirrors,
			},
			nil,
		)
	}
	profileDigest, _ := profile.Digest()
	catalogDigest, _ := distributions.Digest()
	if profileDigest != plan.CurrentProfileDigest || catalogDigest != plan.CatalogDigest {
		return nil, operationError(
			"operator.plan_stale",
			"upgrade",
			"Operator profile or catalog changed after the plan was created",
			map[string]any{
				"plannedProfileDigest": plan.CurrentProfileDigest,
				"currentProfileDigest": profileDigest,
				"plannedCatalogDigest": plan.CatalogDigest,
				"currentCatalogDigest": catalogDigest,
			},
			nil,
		)
	}
	target, err := findDistribution(*distributions, plan.Target.Digest)
	if err != nil || target.State != catalog.StateCertified {
		return nil, operationError("operator.target_changed", "upgrade", "Planned target is no longer certified", map[string]any{"digest": plan.Target.Digest}, err)
	}
	targetDigest, _ := canonical.Digest(target)
	plannedDigest, _ := canonical.Digest(plan.Target)
	if targetDigest != plannedDigest {
		return nil, operationError("operator.target_changed", "upgrade", "Planned target metadata changed", map[string]any{"digest": plan.Target.Digest}, nil)
	}

	state := OperationState{
		Schema:      OperationStateSchema,
		OperationID: plan.OperationID,
		Kind:        "upgrade",
		Operator:    inventory.Operator,
		Status:      "running",
		Phase:       "candidate",
		StartedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Evidence:    map[string]map[string]any{},
	}
	statePath := filepath.Join(inventory.StateDirectory, "operations", plan.OperationID+".json")
	if err := document.WriteAtomic(statePath, state); err != nil {
		return nil, err
	}
	prior := map[string]any{}

	steps := []struct {
		name string
		spec extension.Spec
	}{
		{"candidate", inventory.Upgrade.Candidate},
		{"sync", inventory.Upgrade.Sync},
		{"deploy", inventory.Upgrade.Deploy},
		{"verify", inventory.Upgrade.Verify},
	}
	deployed := false
	for _, step := range steps {
		state.Phase = step.name
		_ = document.WriteAtomic(statePath, state)
		input := UpgradeStepInput{
			Schema:             UpgradeInputSchema,
			OperationID:        plan.OperationID,
			Phase:              step.name,
			Operator:           inventory.Operator,
			Mirrors:            append([]string(nil), plan.Mirrors...),
			CurrentProfile:     *profile,
			TargetDistribution: target,
			PriorEvidence:      cloneEvidence(prior),
		}
		result, stepErr := runStep(ctx, step.name, step.spec, input)
		if stepErr != nil {
			if deployed || step.name == "deploy" {
				stepErr = rollbackUpgrade(ctx, inventory, plan, *profile, target, prior, stepErr)
			}
			recordOperationFailure(statePath, &state, "operator.upgrade_step_failed", step.name, stepErr)
			return nil, operationError("operator.upgrade_step_failed", step.name, "Operator upgrade did not become active", map[string]any{"operationId": plan.OperationID}, stepErr)
		}
		if step.name == "deploy" {
			deployed = true
		}
		prior[step.name] = result.Evidence
		state.Evidence[step.name] = result.Evidence
	}

	targetProfile := *profile
	targetProfile.ServerRelease = operator.ServerRelease{
		Version:            target.Version,
		Commit:             target.Commit,
		DistributionDigest: target.Digest,
	}
	targetProfile.Provides = append(targetProfile.Provides[:0:0], target.Provides...)
	targetProfile.Sources.Mirrors = append([]string(nil), plan.Mirrors...)
	targetProfile.Sequence = profile.Sequence + 1
	keyID, seed, err := loadSigningKey(inventory.SigningKeyPath)
	if err != nil {
		err = rollbackUpgrade(ctx, inventory, plan, *profile, target, prior, err)
		recordOperationFailure(statePath, &state, "operator.profile_sign_failed", "sign-profile", err)
		return nil, operationError("operator.profile_sign_failed", "sign-profile", "New Operator Profile could not be signed", nil, err)
	}
	if err := targetProfile.Sign(keyID, seed); err != nil {
		err = rollbackUpgrade(ctx, inventory, plan, *profile, target, prior, err)
		recordOperationFailure(statePath, &state, "operator.profile_sign_failed", "sign-profile", err)
		return nil, operationError("operator.profile_sign_failed", "sign-profile", "New Operator Profile could not be signed", nil, err)
	}
	stagedProfile := filepath.Join(inventory.StateDirectory, "operations", plan.OperationID+".profile.json")
	if err := document.WriteNew(stagedProfile, targetProfile); err != nil {
		err = rollbackUpgrade(ctx, inventory, plan, *profile, target, prior, err)
		recordOperationFailure(statePath, &state, "operator.profile_stage_failed", "stage-profile", err)
		return nil, operationError("operator.profile_stage_failed", "stage-profile", "New Operator Profile could not be staged", nil, err)
	}

	state.Phase = "activate"
	_ = document.WriteAtomic(statePath, state)
	activateResult, err := runStep(ctx, "activate", inventory.Upgrade.Activate, UpgradeStepInput{
		Schema:             UpgradeInputSchema,
		OperationID:        plan.OperationID,
		Phase:              "activate",
		Operator:           inventory.Operator,
		Mirrors:            append([]string(nil), plan.Mirrors...),
		CurrentProfile:     *profile,
		TargetDistribution: target,
		TargetProfilePath:  stagedProfile,
		PriorEvidence:      cloneEvidence(prior),
	})
	if err != nil {
		err = rollbackUpgrade(ctx, inventory, plan, *profile, target, prior, err)
		recordOperationFailure(statePath, &state, "operator.activation_failed", "activate", err)
		return nil, operationError("operator.activation_failed", "activate", "Operator activation failed; old Profile remains active", nil, err)
	}
	state.Evidence["activate"] = activateResult.Evidence
	if err := replaceFileAtomic(inventory.ActiveProfilePath, stagedProfile); err != nil {
		err = rollbackUpgrade(ctx, inventory, plan, *profile, target, prior, err)
		recordOperationFailure(statePath, &state, "operator.profile_commit_failed", "activate", err)
		return nil, operationError("operator.profile_commit_failed", "activate", "Service activation was rolled back because Profile commit failed", nil, err)
	}
	state.Status = "complete"
	state.Phase = "complete"
	state.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := document.WriteAtomic(statePath, state); err != nil {
		return nil, err
	}
	return &targetProfile, nil
}

func selectUpgradeTarget(distributions catalog.Document, to string) (catalog.Distribution, error) {
	if to == "" || to == "latest" {
		production, err := distributions.Production()
		if err != nil {
			return catalog.Distribution{}, err
		}
		if len(production) == 0 {
			return catalog.Distribution{}, fmt.Errorf("catalog has no certified production distribution")
		}
		return production[0], nil
	}
	var found *catalog.Distribution
	for index := range distributions.Distributions {
		candidate := &distributions.Distributions[index]
		if candidate.Version != to || candidate.State != catalog.StateCertified {
			continue
		}
		if found != nil {
			return catalog.Distribution{}, fmt.Errorf("certified version %s is ambiguous; select by immutable digest", to)
		}
		found = candidate
	}
	if found == nil {
		return catalog.Distribution{}, fmt.Errorf("certified version %s is not present", to)
	}
	return *found, nil
}

func findDistribution(distributions catalog.Document, digest string) (catalog.Distribution, error) {
	for _, distribution := range distributions.Distributions {
		if distribution.Digest == digest {
			return distribution, nil
		}
	}
	return catalog.Distribution{}, fmt.Errorf("distribution %s is not present", digest)
}

func rollbackUpgrade(
	ctx context.Context,
	inventory Inventory,
	plan UpgradePlan,
	profile operator.Document,
	target catalog.Distribution,
	prior map[string]any,
	cause error,
) error {
	_, rollbackErr := runStep(ctx, "rollback", inventory.Upgrade.Rollback, UpgradeStepInput{
		Schema:             UpgradeInputSchema,
		OperationID:        plan.OperationID,
		Phase:              "rollback",
		Operator:           inventory.Operator,
		Mirrors:            append([]string(nil), plan.Mirrors...),
		CurrentProfile:     profile,
		TargetDistribution: target,
		PriorEvidence:      cloneEvidence(prior),
	})
	if rollbackErr != nil {
		return operationError(
			"operator.rollback_failed",
			"rollback",
			"Operator upgrade failed and rollback also failed",
			map[string]any{"originalError": cause.Error()},
			rollbackErr,
		)
	}
	return cause
}

func replaceFileAtomic(destination, staged string) error {
	data, err := os.ReadFile(staged)
	if err != nil {
		return err
	}
	var value operator.Document
	if err := document.Decode(data, &value); err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return err
	}
	return document.WriteAtomic(destination, value)
}

func recordOperationFailure(path string, state *OperationState, code, phase string, cause error) {
	state.Status = "failed"
	state.Phase = phase
	state.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	diagnostic := diag.Envelope{
		Code:     code,
		Severity: diag.SeverityError,
		Stage:    phase,
		Message:  cause.Error(),
		Evidence: map[string]any{"operationId": state.OperationID},
	}
	state.Diagnostic = &diagnostic
	_ = document.WriteAtomic(path, *state)
}

func operationError(code, stage, message string, evidence map[string]any, cause error) error {
	return diag.Error{
		Diagnostic: diag.Envelope{
			Code:     code,
			Severity: diag.SeverityError,
			Stage:    stage,
			Message:  message,
			Evidence: evidence,
		},
		Cause: cause,
	}
}

func newOperationID(prefix string) (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(random[:]), nil
}

func cloneEvidence(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
