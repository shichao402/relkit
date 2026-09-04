package onboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cnb.cool/shichao402/relkit/internal/config"
)

type EvalOptions struct {
	HostRoot      string
	AgentConfig   string
	Product       string
	Now           time.Time
	SkipRemote    bool
	CertTimerPath string
	HTTPGet       func(url string) ([]byte, error)
	Systemctl     func(args ...string) (string, error)
	CertDays      func(domain string) (int, error)
}

func Evaluate(c Checklist, opts EvalOptions) (*Report, error) {
	if err := ValidateChecklist(c); err != nil {
		return nil, err
	}
	hostRoot := opts.HostRoot
	if hostRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		hostRoot = cwd
	}
	hostRoot, err := filepath.Abs(hostRoot)
	if err != nil {
		return nil, err
	}

	cfg, cfgErr := loadHostConfig(hostRoot)
	acks, _ := LoadAcks(hostRoot)
	inputsHash := InputsHash(cfg)
	if acks != nil && acks.InputsHash != "" && acks.InputsHash != inputsHash {
		acks.Acks = map[string]Ack{}
	}

	results := map[string]ItemResult{}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	for _, item := range topoOrder(c) {
		res := ItemResult{ID: item.ID, Title: item.Title, Type: item.Type}
		var blocked []string
		for _, dep := range item.DependsOn {
			parent := results[dep]
			if parent.Status != StatusPassed && parent.Status != StatusNA {
				blocked = append(blocked, dep)
			}
		}
		if len(blocked) > 0 {
			res.Status = StatusBlocked
			res.BlockedBy = blocked
			res.Evidence = "dependencies not passed"
			results[item.ID] = res
			continue
		}

		switch item.Type {
		case TypeManual:
			if acks != nil {
				if ack, ok := acks.Acks[item.ID]; ok && acks.InputsHash == inputsHash {
					res.Status = StatusPassed
					res.Evidence = "acked " + ack.At
					results[item.ID] = res
					continue
				}
			}
			res.Status = StatusPending
			res.Remediation = "relkit onboard ack " + item.ID
			if item.Prompt != "" {
				res.Evidence = item.Prompt
			}
		case TypeExec:
			if item.Exec != nil && item.Exec.Kind == "write-recovery-embed" {
				if cfg == nil || cfg.Recovery == nil {
					res.Status = StatusFailed
					res.Evidence = "recovery is not configured"
					res.Remediation = "add recovery.message and at least two recovery.links in relkit.json"
				} else if err := WriteRecoveryEmbed(hostRoot, cfg.Recovery); err != nil {
					res.Status = StatusFailed
					res.Evidence = err.Error()
				} else {
					res.Status = StatusPassed
					res.Evidence = "wrote " + RecoveryJSONPath(hostRoot)
				}
			} else {
				res.Status = StatusFailed
				res.Evidence = "unknown exec kind"
			}
		default:
			res = detectItem(item, hostRoot, cfg, cfgErr, opts)
		}
		results[item.ID] = res
	}

	report := &Report{
		Schema:     SchemaReport,
		Checklist:  c.ID,
		HostRoot:   hostRoot,
		ComputedAt: now.Format(time.RFC3339),
	}
	for _, item := range c.Items {
		report.Items = append(report.Items, results[item.ID])
	}
	return report, nil
}

func detectItem(item Item, hostRoot string, cfg *config.Config, cfgErr error, opts EvalOptions) ItemResult {
	res := ItemResult{ID: item.ID, Title: item.Title, Type: item.Type}
	kind := ""
	if item.Detect != nil {
		kind = item.Detect.Kind
	}
	switch kind {
	case "self":
		res.Status = StatusPassed
		res.Evidence = "relkit onboard"
	case "config":
		if cfgErr != nil {
			res.Status = StatusFailed
			res.Evidence = cfgErr.Error()
			res.Remediation = "relkit init --product <id> in the host repository"
			return res
		}
		res.Status = StatusPassed
		res.Evidence = filepath.Join(hostRoot, config.ConfigName)
	case "recovery":
		if cfg == nil || cfg.Recovery == nil {
			res.Status = StatusFailed
			res.Evidence = "recovery is missing"
			res.Remediation = "add recovery.message and at least two official links to relkit.json"
			return res
		}
		res.Status = StatusPassed
		res.Evidence = fmt.Sprintf("%d links", len(cfg.Recovery.Links))
	case "recovery-embed":
		if cfg == nil || cfg.Recovery == nil {
			res.Status = StatusFailed
			res.Evidence = "recovery is missing"
			return res
		}
		ok, evidence := RecoveryEmbedMatches(hostRoot, cfg.Recovery)
		if !ok {
			res.Status = StatusFailed
			res.Evidence = evidence
			res.Remediation = "relkit onboard run repo.recovery-embed"
			return res
		}
		res.Status = StatusPassed
		res.Evidence = evidence
	case "recovery-wired":
		na, ok, evidence := hostWiresRecovery(hostRoot)
		if na {
			res.Status = StatusNA
			res.Evidence = "no Go/Dart/Node host sources in this directory"
			return res
		}
		if !ok {
			res.Status = StatusFailed
			res.Evidence = evidence
			res.Remediation = "assign sdk.Updater.Recovery (or Dart/Node equivalent) from compile-time recovery copy"
			return res
		}
		res.Status = StatusPassed
		res.Evidence = evidence
	case "reuse-cli":
		res.Status = StatusPassed
		res.Evidence = "relkit simulate | relkit publish --dry-run | relkit verify --deep"
	case "verify-deep":
		return detectVerifyDeep(cfg, cfgErr, opts, res)
	case "simulate":
		return detectSimulate(cfg, cfgErr, res)
	case "cas-planned":
		res.Status = StatusNA
		res.Evidence = "CAS ingest is planned and does not block the current whole-tree publish path"
		return res
	case "public-keys":
		if cfg == nil {
			res.Status = StatusFailed
			res.Evidence = "no config"
			return res
		}
		raw, _ := cfg.Signing["publicKeys"].([]any)
		if len(raw) == 0 {
			res.Status = StatusFailed
			res.Evidence = "signing.publicKeys is empty"
			res.Remediation = "relkit keygen --key-id <id> --out keys --update-config"
			return res
		}
		res.Status = StatusPassed
		res.Evidence = fmt.Sprintf("%d public keys", len(raw))
	case "entry-urls":
		if cfg == nil || cfg.Directory == nil || len(cfg.Directory.EntryURLs) == 0 {
			res.Status = StatusFailed
			res.Evidence = "directory.entryUrls is empty"
			res.Remediation = "set directory.entryUrls to the client-embedded directory URLs"
			return res
		}
		res.Status = StatusPassed
		res.Evidence = fmt.Sprintf("%d entry urls", len(cfg.Directory.EntryURLs))
	case "client-contract":
		if cfg == nil {
			res.Status = StatusFailed
			res.Evidence = "no config"
			return res
		}
		ok, evidence := ClientContractMatches(hostRoot, cfg)
		if !ok {
			res.Status = StatusFailed
			res.Evidence = evidence
			res.Remediation = "relkit onboard run repo.client-contract"
			return res
		}
		res.Status = StatusPassed
		res.Evidence = evidence
	case "distinct-buckets":
		if cfg == nil {
			res.Status = StatusFailed
			res.Evidence = "no config"
			return res
		}
		if err := checkDistinctS3Buckets(cfg); err != nil {
			res.Status = StatusFailed
			res.Evidence = err.Error()
			res.Remediation = "give each s3-compatible backend its own bucket; do not register extra domains of the same bucket as extra backends"
			return res
		}
		res.Status = StatusPassed
		res.Evidence = "s3-compatible backends use distinct buckets"
	case "second-s3":
		if cfg == nil {
			res.Status = StatusFailed
			res.Evidence = "no config"
			return res
		}
		n := countS3Backends(cfg.Backends)
		if n < 2 {
			res.Status = StatusNA
			res.Evidence = "single s3-compatible backend; second bucket is optional after verification teardown"
			return res
		}
		res.Status = StatusPassed
		res.Evidence = fmt.Sprintf("%d s3-compatible backends", n)
	case "agent-config":
		path := opts.AgentConfig
		if path == "" {
			path = "relkit-agent.json"
		}
		if _, err := os.Stat(path); err != nil {
			res.Status = StatusFailed
			res.Evidence = err.Error()
			res.Remediation = "install relkit-agent first"
			return res
		}
		res.Status = StatusPassed
		res.Evidence = path
	default:
		if detectAgent(kind, opts, &res) {
			return res
		}
		res.Status = StatusFailed
		res.Evidence = "unknown detect kind " + kind
	}
	return res
}

func loadHostConfig(hostRoot string) (*config.Config, error) {
	path := filepath.Join(hostRoot, config.ConfigName)
	return config.Load(path)
}

func countS3Backends(backends map[string]map[string]any) int {
	n := 0
	for _, backend := range backends {
		typ, _ := backend["type"].(string)
		if typ == "s3-compatible" {
			n++
		}
	}
	return n
}

func checkDistinctS3Buckets(cfg *config.Config) error {
	seen := map[string]string{}
	for name, backend := range cfg.Backends {
		typ, _ := backend["type"].(string)
		if typ != "s3-compatible" {
			continue
		}
		bucket, _ := backend["bucket"].(string)
		region, _ := backend["region"].(string)
		key := strings.ToLower(strings.TrimSpace(bucket) + "|" + strings.TrimSpace(region))
		if bucket == "" {
			return fmt.Errorf("backend %q is s3-compatible but has no bucket", name)
		}
		if other, ok := seen[key]; ok {
			return fmt.Errorf("backends %q and %q share bucket %s in %s; that is one backend, not two", other, name, bucket, region)
		}
		seen[key] = name
	}
	return nil
}

func NextFailed(report *Report) *ItemResult {
	if report == nil {
		return nil
	}
	for i := range report.Items {
		item := &report.Items[i]
		switch item.Status {
		case StatusFailed, StatusPending:
			return item
		}
	}
	return nil
}

func AllGreen(report *Report) bool {
	if report == nil {
		return false
	}
	for _, item := range report.Items {
		if item.Status != StatusPassed && item.Status != StatusNA {
			return false
		}
	}
	return true
}
