package sdk_test

import (
	"context"
	"testing"

	"go.firoyang.com/relkit/internal/inprocess"
	"go.firoyang.com/relkit/sdk"
)

func TestCheckAttachesRecoveryWhenRemoteFails(t *testing.T) {
	u := &inprocess.Updater{
		Product:     "demo",
		Channel:     "stable",
		CurrentCode: 1,
		EntryURLs:   []string{"https://127.0.0.1:1/directory/demo.pb"},
		TrustedKeys: sdk.TrustedKeys{"k1": make([]byte, 32)},
		Recovery: &inprocess.RecoveryHelp{
			Message: "install manually",
			Links:   []inprocess.RecoveryLink{{Label: "GitHub", URL: "https://github.com/example/app/releases"}},
		},
		Policy: sdk.Policy{AfterSuccess: 0, AfterFailure: 0, DocumentTimeout: 1},
	}
	result := u.CheckForce(context.Background(), true)
	if result.Err == nil {
		t.Fatal("expected remote failure")
	}
	if result.Recovery == nil || result.Recovery.Message != "install manually" {
		t.Fatalf("recovery: %+v", result.Recovery)
	}
}
