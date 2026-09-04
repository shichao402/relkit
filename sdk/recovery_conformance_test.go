package sdk_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"cnb.cool/shichao402/relkit/sdk"
)

func TestConformanceOfflineRecovery(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "conformance", "recovery", "offline.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Expect struct {
			Available        bool `json:"available"`
			Fallback         bool `json:"fallback"`
			RecoveryRequired bool `json:"recoveryRequired"`
			Recovery         struct {
				Message string `json:"message"`
				Links   []struct {
					Label string `json:"label"`
					URL   string `json:"url"`
				} `json:"links"`
			} `json:"recovery"`
		} `json:"expect"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	u := &sdk.Updater{
		Product:     "demo",
		Channel:     "stable",
		CurrentCode: 1,
		EntryURLs:   []string{"https://127.0.0.1:1/directory/demo.pb"},
		TrustedKeys: sdk.TrustedKeys{"k1": make([]byte, 32)},
		Recovery: &sdk.RecoveryHelp{
			Message: doc.Expect.Recovery.Message,
			Links:   []sdk.RecoveryLink{{Label: doc.Expect.Recovery.Links[0].Label, URL: doc.Expect.Recovery.Links[0].URL}},
		},
		Policy: sdk.Policy{AfterSuccess: 0, AfterFailure: 0, DocumentTimeout: 1},
	}
	result := u.CheckForce(context.Background(), true)
	if result.Available != nil && doc.Expect.Available == false {
		t.Fatal("must not invent UpdateAvailable")
	}
	if result.Fallback != nil && doc.Expect.Fallback == false {
		t.Fatal("must not invent FallbackRequired")
	}
	if doc.Expect.RecoveryRequired && (result.Recovery == nil || result.Recovery.Message != doc.Expect.Recovery.Message) {
		t.Fatalf("recovery: %+v", result.Recovery)
	}
}
