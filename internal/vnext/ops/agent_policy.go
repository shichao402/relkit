package ops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/shichao402/relkit/internal/fsx"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/resolution"
)

const (
	AgentProductPolicySchema = "relkit.agent-product-policy/1"
	AgentPolicyResultSchema  = "relkit.agent-policy-result/1"
)

type AgentProductPolicy struct {
	DistributionRootFingerprint string `json:"distributionRootFingerprint"`
	CandidateRootFingerprint    string `json:"candidateRootFingerprint,omitempty"`
	OperatorDeploymentID        string `json:"operatorDeploymentId"`
	Lane                        string `json:"lane"`
}

type AgentPolicyDocument struct {
	Schema   string                        `json:"schema"`
	Products map[string]AgentProductPolicy `json:"products"`
}

type AgentPolicyOptions struct {
	Config  string
	Policy  string
	Backup  string
	Execute bool
}

type AgentPolicyResult struct {
	Schema          string   `json:"schema"`
	OK              bool     `json:"ok"`
	Changed         bool     `json:"changed"`
	Config          string   `json:"config"`
	ChangedProducts []string `json:"changedProducts,omitempty"`
	Backup          string   `json:"backup,omitempty"`
	Executed        bool     `json:"executed"`
}

// ApplyAgentPolicy plans or atomically applies the vNext trust, deployment,
// and lane policy for already-provisioned agent products. Existing backend
// credentials and every unrelated agent setting are preserved as opaque JSON.
func ApplyAgentPolicy(options AgentPolicyOptions) (*AgentPolicyResult, error) {
	if options.Config == "" || options.Policy == "" {
		return nil, fmt.Errorf("config and policy are required")
	}
	var policy AgentPolicyDocument
	if err := document.Load(options.Policy, &policy); err != nil {
		return nil, err
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(options.Config)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("agent config %s is not a regular file", options.Config)
	}
	original, err := os.ReadFile(options.Config)
	if err != nil {
		return nil, err
	}
	var config map[string]json.RawMessage
	if err := document.Decode(original, &config); err != nil {
		return nil, fmt.Errorf("%s: %w", options.Config, err)
	}
	rawProducts, ok := config["products"]
	if !ok {
		return nil, fmt.Errorf("agent config has no products object")
	}
	var products map[string]json.RawMessage
	if err := document.Decode(rawProducts, &products); err != nil {
		return nil, fmt.Errorf("agent config products: %w", err)
	}

	var changedProducts []string
	for product, desired := range policy.Products {
		rawProduct, exists := products[product]
		if !exists {
			return nil, fmt.Errorf("agent config does not provision product %q", product)
		}
		var current map[string]json.RawMessage
		if err := document.Decode(rawProduct, &current); err != nil {
			return nil, fmt.Errorf("agent config product %q: %w", product, err)
		}
		changed := setJSONString(current, "distributionRootFingerprint", desired.DistributionRootFingerprint)
		if desired.CandidateRootFingerprint == "" {
			if _, exists := current["candidateRootFingerprint"]; exists {
				delete(current, "candidateRootFingerprint")
				changed = true
			}
		} else if setJSONString(current, "candidateRootFingerprint", desired.CandidateRootFingerprint) {
			changed = true
		}
		if setJSONString(current, "operatorDeploymentId", desired.OperatorDeploymentID) {
			changed = true
		}
		if setJSONString(current, "lane", desired.Lane) {
			changed = true
		}
		encoded, err := json.Marshal(current)
		if err != nil {
			return nil, err
		}
		products[product] = json.RawMessage(encoded)
		if changed {
			changedProducts = append(changedProducts, product)
		}
	}
	sort.Strings(changedProducts)
	encodedProducts, err := json.Marshal(products)
	if err != nil {
		return nil, err
	}
	config["products"] = json.RawMessage(encodedProducts)
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	updated = append(updated, '\n')
	result := &AgentPolicyResult{
		Schema: AgentPolicyResultSchema, OK: true, Changed: len(changedProducts) > 0,
		Config: options.Config, ChangedProducts: changedProducts, Executed: options.Execute,
	}
	if !options.Execute || !result.Changed {
		return result, nil
	}
	if options.Backup == "" {
		return nil, fmt.Errorf("backup is required with execute when the policy changes")
	}
	if samePath(options.Config, options.Backup) {
		return nil, fmt.Errorf("backup must not be the agent config path")
	}
	if err := writeBackup(options.Backup, original); err != nil {
		return nil, err
	}
	if err := writeAtomicBytes(options.Config, updated, info.Mode().Perm()); err != nil {
		return nil, err
	}
	result.Backup = options.Backup
	return result, nil
}

func (p AgentPolicyDocument) Validate() error {
	if p.Schema != AgentProductPolicySchema {
		return fmt.Errorf("policy schema is %q, expected %q", p.Schema, AgentProductPolicySchema)
	}
	if len(p.Products) == 0 {
		return fmt.Errorf("policy products must not be empty")
	}
	for product, value := range p.Products {
		if product == "" || value.OperatorDeploymentID == "" {
			return fmt.Errorf("policy products require a product name and operatorDeploymentId")
		}
		if err := canonical.ValidateDigest(value.DistributionRootFingerprint); err != nil {
			return fmt.Errorf("product %s distributionRootFingerprint: %w", product, err)
		}
		switch resolution.Identity(value.Lane) {
		case resolution.IdentityProduction:
		case resolution.IdentityCandidate:
			if value.CandidateRootFingerprint == "" {
				return fmt.Errorf("product %s candidate lane requires candidateRootFingerprint", product)
			}
		default:
			return fmt.Errorf("product %s lane must be production or candidate", product)
		}
		if value.CandidateRootFingerprint != "" {
			if err := canonical.ValidateDigest(value.CandidateRootFingerprint); err != nil {
				return fmt.Errorf("product %s candidateRootFingerprint: %w", product, err)
			}
		}
	}
	return nil
}

func setJSONString(object map[string]json.RawMessage, key, desired string) bool {
	var current string
	if raw, ok := object[key]; ok {
		if err := json.Unmarshal(raw, &current); err == nil && current == desired {
			return false
		}
	}
	object[key], _ = json.Marshal(desired)
	return true
}

func writeBackup(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("backup %s already exists", path)
		}
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}

func writeAtomicBytes(path string, data []byte, mode os.FileMode) error {
	parent := filepath.Dir(path)
	temporary, err := os.CreateTemp(parent, "."+filepath.Base(path)+".")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return fsx.Replace(temporaryPath, path)
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}
