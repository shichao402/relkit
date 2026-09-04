package onboard

func HostChecklist() Checklist {
	return Checklist{
		Schema: SchemaChecklist,
		ID:     "toolchain",
		Items: []Item{
			{ID: "cli.present", Title: "relkit CLI is this binary", Type: TypeDetect, Detect: &Detect{Kind: "self"}},
			{ID: "repo.relkit-json", Title: "relkit.json loads", Type: TypeDetect, DependsOn: []string{"cli.present"}, Detect: &Detect{Kind: "config"}},
			{ID: "repo.recovery", Title: "recovery copy and two official links", Type: TypeDetect, DependsOn: []string{"repo.relkit-json"}, Detect: &Detect{Kind: "recovery"}},
			{ID: "repo.recovery-embed", Title: ".relkit/recovery.json matches relkit.json", Type: TypeDetect, DependsOn: []string{"repo.recovery"}, Detect: &Detect{Kind: "recovery-embed"}},
			{ID: "host.recovery-wired", Title: "host assigns compile-time RecoveryHelp", Type: TypeDetect, DependsOn: []string{"repo.recovery"}, Detect: &Detect{Kind: "recovery-wired"}},
			{ID: "gold.reuse-cli", Title: "simulate / publish --dry-run / verify --deep are the publish tools", Type: TypeDetect, DependsOn: []string{"cli.present"}, Detect: &Detect{Kind: "reuse-cli"}},
			{ID: "gold.verify-deep", Title: "verify --deep against publishTo (or N/A before first publish)", Type: TypeDetect, DependsOn: []string{"repo.relkit-json"}, Detect: &Detect{Kind: "verify-deep"}},
			{ID: "gold.simulate", Title: "simulate can load an index (empty is ok)", Type: TypeDetect, DependsOn: []string{"repo.relkit-json"}, Detect: &Detect{Kind: "simulate"}},
			{ID: "gold.cas-planned", Title: "CAS ingest (planned)", Type: TypeDetect, Detect: &Detect{Kind: "cas-planned"}},
			{ID: "repo.public-keys", Title: "signing.publicKeys present", Type: TypeDetect, DependsOn: []string{"repo.relkit-json"}, Detect: &Detect{Kind: "public-keys"}},
			{ID: "repo.entry-urls", Title: "directory.entryUrls is non-empty", Type: TypeDetect, DependsOn: []string{"repo.relkit-json"}, Detect: &Detect{Kind: "entry-urls"}},
			{ID: "repo.client-contract", Title: ".relkit/client-contract.json matches relkit.json", Type: TypeDetect, DependsOn: []string{"repo.recovery", "repo.public-keys", "repo.entry-urls"}, Detect: &Detect{Kind: "client-contract"}},
			{ID: "human.no-private-key-in-git", Title: "private keys are not committed", Type: TypeManual, DependsOn: []string{"repo.public-keys"}, Prompt: "Confirm private keys are gitignored and never committed"},
			{ID: "backends.distinct-buckets", Title: "each s3-compatible backend uses a distinct bucket", Type: TypeDetect, DependsOn: []string{"repo.relkit-json"}, Detect: &Detect{Kind: "distinct-buckets"}},
			{ID: "backends.second-s3", Title: "a second distinct-bucket s3 backend is registered (or N/A)", Type: TypeDetect, DependsOn: []string{"backends.distinct-buckets"}, Detect: &Detect{Kind: "second-s3"}},
		},
	}
}

func AgentChecklist() Checklist {
	return Checklist{
		Schema: SchemaChecklist,
		ID:     "agent-machine",
		Items: []Item{
			{ID: "agent.config", Title: "relkit-agent.json loads", Type: TypeDetect, Detect: &Detect{Kind: "agent-config"}},
			{ID: "agent.no-instance-token", Title: "no instance-wide upload token", Type: TypeDetect, DependsOn: []string{"agent.config"}, Detect: &Detect{Kind: "agent-no-instance-token"}},
			{ID: "agent.product", Title: "product is registered", Type: TypeDetect, DependsOn: []string{"agent.config"}, Detect: &Detect{Kind: "agent-product"}},
			{ID: "agent.profile", Title: "publish profile is readable", Type: TypeDetect, DependsOn: []string{"agent.product"}, Detect: &Detect{Kind: "agent-profile"}},
			{ID: "agent.token-file", Title: "product upload token file exists", Type: TypeDetect, DependsOn: []string{"agent.product"}, Detect: &Detect{Kind: "agent-token"}},
			{ID: "agent.second-s3", Title: "profile has a second distinct-bucket s3 backend (or N/A)", Type: TypeDetect, DependsOn: []string{"agent.profile"}, Detect: &Detect{Kind: "agent-second-s3"}},
			{ID: "agent.s3-get", Title: "each s3-compatible backend baseUrl answers anonymous GET", Type: TypeDetect, DependsOn: []string{"agent.profile"}, Detect: &Detect{Kind: "agent-s3-get"}},
			{ID: "agent.directory-match", Title: "directory.pb bytes match and verify across entryUrls", Type: TypeDetect, DependsOn: []string{"agent.s3-get"}, Detect: &Detect{Kind: "agent-directory-match"}},
			{ID: "agent.cert-timer", Title: "COS cert renew timer enabled with recent success (or N/A)", Type: TypeDetect, DependsOn: []string{"agent.config"}, Detect: &Detect{Kind: "agent-cert-timer"}},
			{ID: "agent.cert-days", Title: "custom-domain certificates have remaining validity", Type: TypeDetect, DependsOn: []string{"agent.profile"}, Detect: &Detect{Kind: "agent-cert-days"}},
		},
	}
}
