package stagedput

import "testing"

func TestConcurrencyFromEnvUsesFallbackWhenUnset(t *testing.T) {
	t.Setenv("RELKIT_UPLOAD_CONCURRENCY", "")
	if got := ConcurrencyFromEnv(4); got != 4 {
		t.Fatalf("got %d", got)
	}
}

func TestConcurrencyFromEnvReadsValue(t *testing.T) {
	t.Setenv("RELKIT_UPLOAD_CONCURRENCY", "2")
	if got := ConcurrencyFromEnv(4); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := DefaultConcurrencyFromEnv(); got != 2 {
		t.Fatalf("default helper got %d", got)
	}
}
