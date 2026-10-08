package consume_test

import (
	"reflect"
	"testing"

	"github.com/shichao402/relkit/internal/consume"
)

func TestFilterComponentsBySdks(t *testing.T) {
	detected := []string{"sdk-dart", "sdk-go", "sdk-rust", "bindings-ts", "cli", "updater"}

	if got := consume.FilterComponentsBySdks(detected, nil); !reflect.DeepEqual(got, detected) {
		t.Errorf("nil sdks must keep the list unchanged, got %v", got)
	}
	if got := consume.FilterComponentsBySdks(detected, []string{}); !reflect.DeepEqual(got, detected) {
		t.Errorf("empty sdks must keep the list unchanged, got %v", got)
	}
	// Rust only: dart/go SDKs and the TS bindings drop out; cli/updater stay.
	want := []string{"sdk-rust", "cli", "updater"}
	if got := consume.FilterComponentsBySdks(detected, []string{"rust"}); !reflect.DeepEqual(got, want) {
		t.Errorf("sdks=[rust] = %v, want %v", got, want)
	}
	// Rust + node: the bindings survive alongside the rust SDK.
	want = []string{"sdk-rust", "bindings-ts", "cli", "updater"}
	if got := consume.FilterComponentsBySdks(detected, []string{"rust", "node"}); !reflect.DeepEqual(got, want) {
		t.Errorf("sdks=[rust,node] = %v, want %v", got, want)
	}
	// host-scripts (product-tree without UpdaterProcess) always passes.
	withHost := append([]string{"host-scripts"}, detected...)
	got := consume.FilterComponentsBySdks(withHost, []string{"rust"})
	if len(got) == 0 || got[0] != "host-scripts" {
		t.Errorf("host-scripts must survive sdks filtering, got %v", got)
	}
}

func TestValidateSdks(t *testing.T) {
	if out, err := consume.ValidateSdks(nil); err != nil || out != nil {
		t.Errorf("nil input must pass through, got %v %v", out, err)
	}
	if out, err := consume.ValidateSdks([]string{"Rust", " rust ", "NODE"}); err != nil ||
		!reflect.DeepEqual(out, []string{"rust", "node"}) {
		t.Errorf("case/duplicate normalization failed: %v %v", out, err)
	}
	if _, err := consume.ValidateSdks([]string{"python"}); err == nil {
		t.Error("unknown language must be rejected")
	}
	if _, err := consume.ValidateSdks([]string{"  "}); err == nil {
		t.Error("whitespace-only input must be rejected as empty")
	}
}
