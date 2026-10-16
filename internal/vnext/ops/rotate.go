package ops

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/extension"
	"github.com/shichao402/relkit/internal/vnext/operator"
)

type RotateStepInput struct {
	Schema        string         `json:"schema"`
	OperationID   string         `json:"operationId"`
	Phase         string         `json:"phase"`
	Operator      string         `json:"operator"`
	Credential    string         `json:"credential"`
	OldSecretRef  string         `json:"oldSecretRef"`
	NewSecretRef  string         `json:"newSecretRef,omitempty"`
	PriorEvidence map[string]any `json:"priorEvidence,omitempty"`
}

func RotateCredential(ctx context.Context, inventory Inventory, credential string) (*operator.Document, error) {
	workflow, ok := inventory.Rotate[credential]
	if !ok {
		return nil, operationError("rotate.credential_unknown", "rotate-plan", "Credential is not declared in Operator inventory", map[string]any{"credential": credential}, nil)
	}
	profile, _, _, err := loadTrustedState(inventory)
	if err != nil {
		return nil, operationError("operator.control_plane_invalid", "rotate-plan", "Operator control plane is not trusted", nil, err)
	}
	oldRef := profile.SecretRefs[credential]
	if oldRef == "" {
		return nil, operationError("rotate.secret_ref_missing", "rotate-plan", "Active Operator Profile has no secret reference for this credential", map[string]any{"credential": credential}, nil)
	}
	operationID, err := newOperationID("rotate")
	if err != nil {
		return nil, err
	}
	state := OperationState{
		Schema:      OperationStateSchema,
		OperationID: operationID,
		Kind:        "rotate",
		Operator:    inventory.Operator,
		Status:      "running",
		Phase:       "mint",
		StartedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Evidence:    map[string]map[string]any{},
	}
	statePath := filepath.Join(inventory.StateDirectory, "operations", operationID+".json")
	if err := document.WriteAtomic(statePath, state); err != nil {
		return nil, err
	}
	prior := map[string]any{}
	newRef := ""
	var targetProfile *operator.Document
	steps := []struct {
		name string
		spec extension.Spec
	}{
		{"mint", workflow.Mint},
		{"update-secret-store", workflow.Update},
		{"check-service-read", workflow.CheckRead},
		{"reload", workflow.Reload},
		{"health-and-write", workflow.Health},
		{"revoke-old", workflow.Revoke},
	}
	for _, step := range steps {
		state.Phase = step.name
		_ = document.WriteAtomic(statePath, state)
		result, stepErr := runStep(ctx, step.name, step.spec, RotateStepInput{
			Schema:        RotateInputSchema,
			OperationID:   operationID,
			Phase:         step.name,
			Operator:      inventory.Operator,
			Credential:    credential,
			OldSecretRef:  oldRef,
			NewSecretRef:  newRef,
			PriorEvidence: cloneEvidence(prior),
		})
		if stepErr != nil {
			recordOperationFailure(statePath, &state, rotateFailureCode(step.name), step.name, stepErr)
			return nil, operationError(
				rotateFailureCode(step.name),
				step.name,
				"Credential rotation is not active; the old Profile remains authoritative",
				map[string]any{"operationId": operationID, "credential": credential},
				stepErr,
			)
		}
		if step.name == "mint" {
			if result.SecretRef == "" || result.SecretRef == oldRef {
				err := fmt.Errorf("mint adapter must return a new opaque secretRef")
				recordOperationFailure(statePath, &state, "rotate.mint_invalid", step.name, err)
				return nil, operationError("rotate.mint_invalid", step.name, "Credential rotation did not mint a new secret reference", nil, err)
			}
			newRef = result.SecretRef
			target := *profile
			target.SecretRefs = cloneStrings(profile.SecretRefs)
			target.SecretRefs[credential] = newRef
			target.Sequence = profile.Sequence + 1
			keyID, seed, signErr := loadSigningKey(inventory.SigningKeyPath)
			if signErr == nil {
				signErr = target.Sign(keyID, seed)
			}
			if signErr != nil {
				recordOperationFailure(statePath, &state, "rotate.profile_sign_failed", "sign-profile", signErr)
				return nil, operationError("rotate.profile_sign_failed", "sign-profile", "New credential was minted but the next Operator Profile could not be signed; the old credential remains active", nil, signErr)
			}
			stagedProfile := filepath.Join(inventory.StateDirectory, "operations", operationID+".profile.json")
			if signErr = document.WriteNew(stagedProfile, target); signErr != nil {
				recordOperationFailure(statePath, &state, "rotate.profile_stage_failed", "stage-profile", signErr)
				return nil, operationError("rotate.profile_stage_failed", "stage-profile", "New credential was minted but the next Operator Profile could not be staged; the old credential remains active", nil, signErr)
			}
			targetProfile = &target
		} else if result.SecretRef != "" && result.SecretRef != newRef {
			err := fmt.Errorf("%s adapter changed secretRef from %q to %q", step.name, newRef, result.SecretRef)
			recordOperationFailure(statePath, &state, "rotate.secret_ref_changed", step.name, err)
			return nil, operationError("rotate.secret_ref_changed", step.name, "Credential adapter changed the minted secret reference", nil, err)
		}
		prior[step.name] = result.Evidence
		state.Evidence[step.name] = result.Evidence
	}

	if targetProfile == nil {
		err := fmt.Errorf("rotation completed without a staged target profile")
		recordOperationFailure(statePath, &state, "rotate.profile_stage_missing", "commit-profile", err)
		return nil, operationError("rotate.profile_stage_missing", "commit-profile", "Rotated credential has no staged Operator Profile", nil, err)
	}
	if err := document.WriteAtomic(inventory.ActiveProfilePath, *targetProfile); err != nil {
		recordOperationFailure(statePath, &state, "rotate.profile_commit_failed", "commit-profile", err)
		return nil, operationError("rotate.profile_commit_failed", "commit-profile", "Rotated credential is live but the Operator Profile was not activated; manual recovery is required", nil, err)
	}
	state.Status = "complete"
	state.Phase = "complete"
	state.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := document.WriteAtomic(statePath, state); err != nil {
		return nil, err
	}
	return targetProfile, nil
}

func rotateFailureCode(phase string) string {
	switch phase {
	case "mint":
		return "rotate.mint_failed"
	case "update-secret-store":
		return "rotate.secret_store_failed"
	case "check-service-read":
		return "rotate.service_read_failed"
	case "reload":
		return "rotate.reload_failed"
	case "health-and-write":
		return "rotate.health_failed"
	case "revoke-old":
		return "rotate.revoke_failed"
	default:
		return "rotate.step_failed"
	}
}

func cloneStrings(value map[string]string) map[string]string {
	result := make(map[string]string, len(value)+1)
	for key, item := range value {
		result[key] = item
	}
	return result
}
