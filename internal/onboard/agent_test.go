package onboard_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/envelope"
	"cnb.cool/shichao402/relkit/internal/keys"
	"cnb.cool/shichao402/relkit/internal/onboard"
)

func TestAgentChecklistRemoteMatch(t *testing.T) {
	seed, err := keys.GenerateSeed()
	if err != nil {
		t.Fatal(err)
	}
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	dirDoc := &rupv2.UpdateDirectory{
		Schema: rupv2.SchemaDirectory, Product: "demo", DirectorySequence: 1,
		UpdatedAt: "2026-08-01T00:00:00Z",
		Services: []*rupv2.DirectoryService{{
			Id: "primary", Priority: 10, IndexUrl: "https://example.invalid/index.pb", Channel: "stable",
		}},
	}
	env, err := envelope.SealDirectory(dirDoc, []envelope.Signer{{KeyID: "k1", Seed: seed}})
	if err != nil {
		t.Fatal(err)
	}
	body, err := rupv2.MarshalEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}

	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer s1.Close()
	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer s2.Close()

	dir := t.TempDir()
	agent := filepath.Join(dir, "relkit-agent.json")
	profile := filepath.Join(dir, "products", "demo.json")
	token := filepath.Join(dir, "tokens", "demo.token")
	_ = os.MkdirAll(filepath.Dir(profile), 0o755)
	_ = os.MkdirAll(filepath.Dir(token), 0o755)
	_ = os.WriteFile(token, []byte("tok"), 0o600)
	_ = os.WriteFile(agent, []byte(`{
		"uploadTokens": [{"file":"tokens/demo.token","products":["demo"]}],
		"products": {"demo": {"root": ".", "profile": "products/demo.json"}}
	}`), 0o644)
	_ = os.WriteFile(profile, []byte(`{
		"backends": {
			"cos": {"type":"s3-compatible","bucket":"a","region":"ap-guangzhou","baseUrl":"`+s1.URL+`/rup/"},
			"cos2": {"type":"s3-compatible","bucket":"b","region":"ap-chengdu","baseUrl":"`+s2.URL+`/rup/"}
		},
		"directory": {"entryUrls":["`+s1.URL+`/rup/directory/demo.pb","`+s2.URL+`/rup/directory/demo.pb"]}
	}`), 0o644)
	relkit := `{
		"product":"demo","defaultChannel":"stable","channels":["stable"],"codeStrategy":"explicit",
		"signing":{"keyId":"k1","privateKeyPath":"k.pb","publicKeys":[{"keyId":"k1","publicKeyBase64":"` + base64.StdEncoding.EncodeToString(pub) + `"}]},
		"backends":{"local":{"type":"local","outputDir":"dist","baseUrl":"https://example.invalid/rup/"}},
		"publishTo":["local"],
		"recovery":{"message":"x","links":[{"label":"a","url":"https://example.com/a"},{"label":"b","url":"https://example.com/b"}]}
	}`
	if err := os.WriteFile(filepath.Join(dir, "relkit.json"), []byte(relkit), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := onboard.Evaluate(onboard.AgentChecklist(), onboard.EvalOptions{
		AgentConfig: agent,
		Product:     "demo",
		CertDays:    func(string) (int, error) { return 40, nil },
		Systemctl:   func(...string) (string, error) { return "", os.ErrNotExist },
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]onboard.ItemResult{}
	for _, item := range report.Items {
		byID[item.ID] = item
	}
	if byID["agent.s3-get"].Status != onboard.StatusPassed {
		t.Fatalf("s3-get: %+v", byID["agent.s3-get"])
	}
	if byID["agent.directory-match"].Status != onboard.StatusPassed {
		t.Fatalf("directory-match: %+v", byID["agent.directory-match"])
	}
	if byID["agent.second-s3"].Status != onboard.StatusPassed {
		t.Fatalf("second-s3: %+v", byID["agent.second-s3"])
	}
	if byID["agent.cert-days"].Status != onboard.StatusPassed {
		t.Fatalf("cert-days: %+v", byID["agent.cert-days"])
	}
}

func TestAgentOnboardMissingConfig(t *testing.T) {
	report, err := onboard.Evaluate(onboard.AgentChecklist(), onboard.EvalOptions{
		AgentConfig: filepath.Join(t.TempDir(), "missing.json"),
		Product:     "demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Items[0].Status != onboard.StatusFailed {
		t.Fatalf("expected config fail: %+v", report.Items[0])
	}
}

func TestAckOnlyManual(t *testing.T) {
	if onboard.IsManualItem(onboard.HostChecklist(), "repo.relkit-json") {
		t.Fatal("detect item must not be manual")
	}
	if !onboard.IsManualItem(onboard.HostChecklist(), "human.no-private-key-in-git") {
		t.Fatal("expected manual item")
	}
}
