package sdk

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func TestRuntimeBootstrapFromJSON(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	root := "sha256:" + strings.Repeat("b", 64)
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	source := fmt.Sprintf(`{
		"schema":"relkit.runtime-bootstrap/1",
		"resolutionDigest":%q,
		"profileDigest":%q,
		"rootFingerprint":%q,
		"profile":{
			"product":"demo",
			"allowedChannels":["stable"],
			"directoryBootstrap":{
				"entryUrls":["https://updates.example/directory/demo.pb"],
				"recovery":{"message":"offline","links":[{"label":"download","url":"https://updates.example/"}]}
			},
			"digest":%q
		},
		"trustedKeys":[{"keyId":"root","key":%q}]
	}`, digest, digest, root, digest, key)
	value, err := RuntimeBootstrapFromJSON([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if value.ClientProfile.GetProduct() != "demo" {
		t.Fatalf("product = %q", value.ClientProfile.GetProduct())
	}
	if got := value.ClientProfile.GetTrustedKeys()[0].GetPublicKey(); len(got) != 32 {
		t.Fatalf("key length = %d", len(got))
	}
	if value.ClientProfile.GetRecovery().GetMessage() != "offline" {
		t.Fatalf("recovery = %q", value.ClientProfile.GetRecovery().GetMessage())
	}
}

func TestRuntimeBootstrapFromJSONRejectsMissingContract(t *testing.T) {
	if _, err := RuntimeBootstrapFromJSON([]byte(`{}`)); err == nil {
		t.Fatal("expected invalid bootstrap to fail")
	}
}
