package model

import "testing"

func TestCasKey(t *testing.T) {
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	got, err := CasKey(digest)
	if err != nil {
		t.Fatal(err)
	}
	if got != "cas/"+digest {
		t.Fatalf("got %q", got)
	}
	if _, err := CasKey("short"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := CasKey(digest[:63] + "G"); err == nil {
		t.Fatal("expected hex error")
	}
}
