package canonical

import (
	"strings"
	"testing"
)

func TestBytesAndDigestAreStableAcrossMapOrder(t *testing.T) {
	left := map[string]any{
		"z": 2,
		"a": map[string]any{"y": true, "x": "value"},
	}
	right := map[string]any{
		"a": map[string]any{"x": "value", "y": true},
		"z": 2,
	}

	leftBytes, err := Bytes(left)
	if err != nil {
		t.Fatal(err)
	}
	rightBytes, err := Bytes(right)
	if err != nil {
		t.Fatal(err)
	}
	if string(leftBytes) != string(rightBytes) {
		t.Fatalf("canonical bytes differ:\n%s\n%s", leftBytes, rightBytes)
	}
	if got, want := string(leftBytes), `{"a":{"x":"value","y":true},"z":2}`; got != want {
		t.Fatalf("canonical bytes = %s, want %s", got, want)
	}

	leftDigest, err := Digest(left)
	if err != nil {
		t.Fatal(err)
	}
	rightDigest, err := Digest(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftDigest != rightDigest {
		t.Fatalf("digest differs: %s != %s", leftDigest, rightDigest)
	}
	if err := ValidateDigest(leftDigest); err != nil {
		t.Fatal(err)
	}
}

func TestValidateDigestRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{"", "sha1:abc", "sha256:abc", "sha256:" + strings.Repeat("z", 64)} {
		if err := ValidateDigest(value); err == nil {
			t.Fatalf("ValidateDigest(%q) unexpectedly succeeded", value)
		}
	}
}
