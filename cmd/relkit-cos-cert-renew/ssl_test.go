package main

import (
	"strings"
	"testing"
)

func TestTC3AuthorizationPrefix(t *testing.T) {
	got := tc3Sign("ssl", "ssl.tencentcloudapi.com", "{}", "1710000000", "2024-03-09", "AKI", "SECRET")
	if !strings.HasPrefix(got, "TC3-HMAC-SHA256 Credential=AKI/2024-03-09/ssl/tc3_request") {
		t.Fatalf("got %s", got)
	}
	if !strings.Contains(got, "SignedHeaders=content-type;host") {
		t.Fatalf("got %s", got)
	}
}
