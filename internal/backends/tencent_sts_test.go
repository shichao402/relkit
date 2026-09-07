package backends

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetFederationTokenParsesCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TC-Action") != "GetFederationToken" {
			t.Errorf("action=%q", r.Header.Get("X-TC-Action"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "TC3-HMAC-SHA256 ") {
			t.Errorf("authorization=%q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		var req tencentSTSRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Error(err)
		}
		if req.Name != "relkit-cas" || req.DurationSeconds < 1800 {
			t.Errorf("request=%+v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Response": map[string]any{
				"Credentials": map[string]any{
					"Token": "tok", "TmpSecretId": "AKIATMP", "TmpSecretKey": "tmp-secret",
				},
				"ExpiredTime": time.Now().Add(time.Hour).Unix(),
				"Expiration":  time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			},
		})
	}))
	defer server.Close()
	prev := tencentSTSEndpoint
	tencentSTSEndpoint = server.URL
	defer func() { tencentSTSEndpoint = prev }()

	creds, err := getFederationToken(server.Client(), "AKID", "SECRET", "relkit-cas", `{"version":"2.0"}`, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if creds.SecretID != "AKIATMP" || creds.Token != "tok" {
		t.Fatalf("creds=%+v", creds)
	}
}

func TestAppIDFromBucket(t *testing.T) {
	if got := appIDFromBucket("relkit-updates-1251882798"); got != "1251882798" {
		t.Fatalf("got %q", got)
	}
	if got := appIDFromBucket("plain"); got != "" {
		t.Fatalf("got %q", got)
	}
}
