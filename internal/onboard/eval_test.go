package onboard_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/jsonio"
	"cnb.cool/shichao402/relkit/internal/onboard"
)

func TestValidateChecklistRejectsCycle(t *testing.T) {
	err := onboard.ValidateChecklist(onboard.Checklist{
		Schema: onboard.SchemaChecklist,
		ID:     "x",
		Items: []onboard.Item{
			{ID: "a", Title: "a", Type: onboard.TypeDetect, DependsOn: []string{"b"}, Detect: &onboard.Detect{Kind: "self"}},
			{ID: "b", Title: "b", Type: onboard.TypeDetect, DependsOn: []string{"a"}, Detect: &onboard.Detect{Kind: "self"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("got %v", err)
	}
}

func TestDecodeChecklistRejectsUnknownField(t *testing.T) {
	_, err := onboard.DecodeChecklist([]byte(`{"schema":"relkit.onboard-checklist/1","id":"x","unknown":true,"items":[]}`))
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("got %v", err)
	}
}

func TestEvaluateBlocksDependents(t *testing.T) {
	dir := t.TempDir()
	report, err := onboard.Evaluate(onboard.HostChecklist(), onboard.EvalOptions{HostRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]onboard.ItemResult{}
	for _, item := range report.Items {
		byID[item.ID] = item
	}
	if byID["repo.relkit-json"].Status != onboard.StatusFailed {
		t.Fatalf("config: %+v", byID["repo.relkit-json"])
	}
	if byID["repo.recovery"].Status != onboard.StatusBlocked {
		t.Fatalf("recovery should be blocked, got %+v", byID["repo.recovery"])
	}
}

func demoDoc() map[string]any {
	doc := config.Skeleton("demo")
	signing, _ := doc["signing"].(map[string]any)
	signing["publicKeys"] = []any{map[string]any{
		"keyId": "k1", "publicKeyBase64": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
	}}
	doc["directory"] = map[string]any{
		"entryUrls": []any{"https://raw.example/rup/directory/demo.pb", "https://raw2.example/rup/directory/demo.pb"},
	}
	return doc
}

func TestEvaluateRecoveryEmbedAndAckHash(t *testing.T) {
	dir := t.TempDir()
	doc := demoDoc()
	raw, err := jsonio.MarshalPretty(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "relkit.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := onboard.Evaluate(onboard.HostChecklist(), onboard.EvalOptions{HostRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]onboard.ItemResult{}
	for _, item := range report.Items {
		byID[item.ID] = item
	}
	if byID["repo.recovery"].Status != onboard.StatusPassed {
		t.Fatalf("recovery: %+v", byID["repo.recovery"])
	}
	if byID["repo.recovery-embed"].Status != onboard.StatusFailed {
		t.Fatalf("embed should fail before write: %+v", byID["repo.recovery-embed"])
	}
	if byID["repo.client-contract"].Status != onboard.StatusFailed {
		t.Fatalf("contract should fail before write: %+v", byID["repo.client-contract"])
	}
	cfg, err := config.Load(filepath.Join(dir, "relkit.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := onboard.WriteRecoveryEmbed(dir, cfg.Recovery); err != nil {
		t.Fatal(err)
	}
	if err := onboard.WriteClientContract(dir, cfg); err != nil {
		t.Fatal(err)
	}
	report, err = onboard.Evaluate(onboard.HostChecklist(), onboard.EvalOptions{HostRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report.Items {
		byID[item.ID] = item
	}
	if byID["repo.recovery-embed"].Status != onboard.StatusPassed {
		t.Fatalf("embed: %+v", byID["repo.recovery-embed"])
	}
	if byID["repo.client-contract"].Status != onboard.StatusPassed {
		t.Fatalf("contract: %+v", byID["repo.client-contract"])
	}

	if err := onboard.SaveAck(dir, "human.no-private-key-in-git", "ok", onboard.InputsHash(cfg)); err != nil {
		t.Fatal(err)
	}
	report, err = onboard.Evaluate(onboard.HostChecklist(), onboard.EvalOptions{HostRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report.Items {
		byID[item.ID] = item
	}
	if byID["human.no-private-key-in-git"].Status != onboard.StatusPassed {
		t.Fatalf("ack: %+v", byID["human.no-private-key-in-git"])
	}

	doc["recovery"] = map[string]any{
		"message": "changed",
		"links":   doc["recovery"].(map[string]any)["links"],
	}
	raw, _ = jsonio.MarshalPretty(doc)
	_ = os.WriteFile(filepath.Join(dir, "relkit.json"), raw, 0o644)
	report, err = onboard.Evaluate(onboard.HostChecklist(), onboard.EvalOptions{HostRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report.Items {
		byID[item.ID] = item
	}
	if byID["human.no-private-key-in-git"].Status == onboard.StatusPassed {
		t.Fatal("ack should invalidate when recovery inputs change")
	}
	if byID["repo.recovery-embed"].Status != onboard.StatusFailed {
		t.Fatal("embed should fail after recovery copy drifts")
	}
}

func TestDistinctBuckets(t *testing.T) {
	dir := t.TempDir()
	doc := demoDoc()
	doc["backends"] = map[string]any{
		"cos": map[string]any{
			"type": "s3-compatible", "bucket": "a-125", "region": "ap-guangzhou",
			"endpoint": "https://cos.ap-guangzhou.myqcloud.com", "prefix": "rup/",
			"baseUrl": "https://raw.example/rup/", "accessKeyEnv": "A", "secretKeyEnv": "B",
		},
		"cos2": map[string]any{
			"type": "s3-compatible", "bucket": "a-125", "region": "ap-guangzhou",
			"endpoint": "https://cos.ap-guangzhou.myqcloud.com", "prefix": "rup/",
			"baseUrl": "https://raw2.example/rup/", "accessKeyEnv": "A", "secretKeyEnv": "B",
		},
	}
	doc["publishTo"] = []any{"cos", "cos2"}
	raw, _ := jsonio.MarshalPretty(doc)
	_ = os.WriteFile(filepath.Join(dir, "relkit.json"), raw, 0o644)
	report, err := onboard.Evaluate(onboard.HostChecklist(), onboard.EvalOptions{HostRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report.Items {
		if item.ID == "backends.distinct-buckets" && item.Status != onboard.StatusFailed {
			t.Fatalf("expected distinct-buckets fail: %+v", item)
		}
	}
}

func TestEvaluateIdempotent(t *testing.T) {
	dir := t.TempDir()
	raw, _ := jsonio.MarshalPretty(config.Skeleton("demo"))
	_ = os.WriteFile(filepath.Join(dir, "relkit.json"), raw, 0o644)
	a, err := onboard.Evaluate(onboard.HostChecklist(), onboard.EvalOptions{HostRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	b, err := onboard.Evaluate(onboard.HostChecklist(), onboard.EvalOptions{HostRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Items) != len(b.Items) {
		t.Fatal("length")
	}
	for i := range a.Items {
		if a.Items[i].Status != b.Items[i].Status || a.Items[i].ID != b.Items[i].ID {
			t.Fatalf("%v vs %v", a.Items[i], b.Items[i])
		}
	}
}

func TestSecondS3NotApplicableForLocalOnly(t *testing.T) {
	dir := t.TempDir()
	raw, _ := jsonio.MarshalPretty(demoDoc())
	_ = os.WriteFile(filepath.Join(dir, "relkit.json"), raw, 0o644)
	report, err := onboard.Evaluate(onboard.HostChecklist(), onboard.EvalOptions{HostRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report.Items {
		if item.ID == "backends.second-s3" && item.Status != onboard.StatusNA {
			t.Fatalf("expected NA, got %+v", item)
		}
	}
}
