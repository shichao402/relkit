package onboard

import (
	"os"
	"path/filepath"
	"strings"

	"cnb.cool/shichao402/relkit/internal/backends"
	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/simulate"
	"cnb.cool/shichao402/relkit/internal/verify"
)

// credentialsElsewhere marks a step that can only run where the publish
// credentials are. Reporting it as failed on a developer machine would train
// operators to ignore red items, since a host checkout is never supposed to
// hold long-lived backend write keys.
func credentialsElsewhere(res ItemResult, what string) ItemResult {
	res.Status = StatusNA
	res.Evidence = "no backend credentials on this machine; they live only on the publish host"
	res.Remediation = "run relkit " + what + " on the publish host"
	return res
}

func detectVerifyDeep(cfg *config.Config, cfgErr error, opts EvalOptions, res ItemResult) ItemResult {
	if cfgErr != nil || cfg == nil {
		res.Status = StatusFailed
		res.Evidence = "relkit.json is not loaded"
		return res
	}
	if opts.SkipRemote {
		res.Status = StatusNA
		res.Evidence = "verify --deep skipped"
		return res
	}
	findings, err := verify.Run(cfg, "", nil, true, func(string) {})
	if err != nil {
		if backends.IsMissingCredentialMessage(err.Error()) {
			return credentialsElsewhere(res, "verify --deep")
		}
		res.Status = StatusFailed
		res.Evidence = err.Error()
		res.Remediation = "relkit verify --deep"
		return res
	}
	if findings.OK() {
		res.Status = StatusPassed
		res.Evidence = "verify --deep passed on publishTo"
		return res
	}
	onlyCredentials := len(findings.Errors) > 0
	for _, msg := range findings.Errors {
		if !backends.IsMissingCredentialMessage(msg) {
			onlyCredentials = false
			break
		}
	}
	if onlyCredentials {
		return credentialsElsewhere(res, "verify --deep")
	}
	onlyMissing := true
	for _, msg := range findings.Errors {
		if !strings.Contains(msg, "no index published") {
			onlyMissing = false
			break
		}
	}
	if onlyMissing && len(findings.Errors) > 0 {
		res.Status = StatusNA
		res.Evidence = "no index published yet; run relkit verify --deep after the first publish"
		return res
	}
	res.Status = StatusFailed
	res.Evidence = strings.Join(findings.Errors, "; ")
	res.Remediation = "relkit verify --deep"
	return res
}

func detectSimulate(cfg *config.Config, cfgErr error, res ItemResult) ItemResult {
	if cfgErr != nil || cfg == nil {
		res.Status = StatusFailed
		res.Evidence = "relkit.json is not loaded"
		return res
	}
	if _, err := os.Stat(filepath.Join(cfg.Root, "VERSION.json")); err != nil {
		res.Status = StatusNA
		res.Evidence = "no VERSION.json yet"
		return res
	}
	if _, err := simulate.LoadIndex(cfg, "", "", "", ""); err != nil {
		if backends.IsMissingCredentialMessage(err.Error()) {
			return credentialsElsewhere(res, "simulate")
		}
		res.Status = StatusFailed
		res.Evidence = err.Error()
		res.Remediation = "relkit simulate"
		return res
	}
	res.Status = StatusPassed
	res.Evidence = "simulate.LoadIndex succeeded (empty index is ok)"
	return res
}

func hostWiresRecovery(root string) (na bool, ok bool, evidence string) {
	hasHost := false
	wired := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		name := info.Name()
		if info.IsDir() {
			switch name {
			case ".git", "node_modules", ".relkit", "vendor", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".test.ts") {
			return nil
		}
		ext := filepath.Ext(name)
		switch ext {
		case ".go", ".dart", ".ts":
		default:
			return nil
		}
		if name == "go.mod" || name == "pubspec.yaml" || name == "package.json" {
			hasHost = true
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		text := string(data)
		if ext == ".go" && (name == "updater.go" && strings.Contains(path, string(filepath.Separator)+"sdk"+string(filepath.Separator))) {
			return nil
		}
		if strings.Contains(text, "sdk.RecoveryHelp") && strings.Contains(text, "Recovery:") {
			hasHost = true
			wired = true
			evidence = path
			return filepath.SkipAll
		}
		if strings.Contains(text, "embeddedRecovery") && strings.Contains(text, "Recovery:") {
			hasHost = true
			wired = true
			evidence = path
			return filepath.SkipAll
		}
		if ext == ".dart" && strings.Contains(text, "RecoveryHelp") && strings.Contains(text, "recovery:") {
			hasHost = true
			wired = true
			evidence = path
			return filepath.SkipAll
		}
		if ext == ".ts" && strings.Contains(text, "RecoveryHelp") && strings.Contains(text, "recovery:") {
			hasHost = true
			wired = true
			evidence = path
			return filepath.SkipAll
		}
		if name == "go.mod" || strings.HasSuffix(path, "pubspec.yaml") {
			hasHost = true
		}
		return nil
	})
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
		hasHost = true
	}
	if _, err := os.Stat(filepath.Join(root, "pubspec.yaml")); err == nil {
		hasHost = true
	}
	if !hasHost {
		return true, false, "no host tree"
	}
	if !wired {
		return false, false, "host sources do not assign compile-time RecoveryHelp"
	}
	return false, true, evidence
}
