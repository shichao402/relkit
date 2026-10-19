package main

import (
	"strings"
	"testing"
)

func TestVNextAgentTransportURLExplicitEndpointOverride(t *testing.T) {
	const signed = "https://operator.example.invalid/"

	t.Run("dry run keeps signed endpoint", func(t *testing.T) {
		t.Setenv("RELKIT_AGENT_TRANSPORT_URL", "https://attacker.example.invalid/")
		got, err := vNextAgentTransportURL(signed, false)
		if err != nil {
			t.Fatal(err)
		}
		if got != signed {
			t.Fatalf("endpoint=%q, want signed endpoint %q", got, signed)
		}
	})

	t.Run("CI may override with a domain endpoint", func(t *testing.T) {
		t.Setenv("RELKIT_RELEASE_VIA_CI", "1")
		t.Setenv("RELKIT_AGENT_TRANSPORT_URL", "http://publish.devcloud.woa.com/")
		got, err := vNextAgentTransportURL(signed, true)
		if err != nil {
			t.Fatal(err)
		}
		if got != "http://publish.devcloud.woa.com/" {
			t.Fatalf("endpoint=%q", got)
		}
	})

	t.Run("CI override does not validate host against the signed endpoint", func(t *testing.T) {
		// The same configuration must work regardless of whether the service
		// instance is co-located with the signed endpoint or not.
		for name, override := range map[string]string{
			"same machine as signed endpoint": "https://operator.example.invalid/",
			"different machine":              "https://publish.example.invalid/",
			"explicit non default port":      "https://publish.example.invalid:8443/",
		} {
			t.Run(name, func(t *testing.T) {
				t.Setenv("RELKIT_RELEASE_VIA_CI", "1")
				t.Setenv("RELKIT_AGENT_TRANSPORT_URL", override)
				got, err := vNextAgentTransportURL(signed, true)
				if err != nil {
					t.Fatalf("override %q rejected: %v", override, err)
				}
				if got != override {
					t.Fatalf("endpoint=%q, want %q", got, override)
				}
			})
		}
	})

	t.Run("CI may still use IPv4 loopback", func(t *testing.T) {
		t.Setenv("RELKIT_RELEASE_VIA_CI", "1")
		t.Setenv("RELKIT_AGENT_TRANSPORT_URL", "http://127.0.0.1:18787/")
		got, err := vNextAgentTransportURL(signed, true)
		if err != nil {
			t.Fatal(err)
		}
		if got != "http://127.0.0.1:18787/" {
			t.Fatalf("endpoint=%q", got)
		}
	})

	t.Run("CI may still use IPv6 loopback", func(t *testing.T) {
		t.Setenv("RELKIT_RELEASE_VIA_CI", "1")
		t.Setenv("RELKIT_AGENT_TRANSPORT_URL", "https://[::1]:18787/v1")
		got, err := vNextAgentTransportURL(signed, true)
		if err != nil {
			t.Fatal(err)
		}
		if got != "https://[::1]:18787/v1" {
			t.Fatalf("endpoint=%q", got)
		}
	})

	t.Run("empty override keeps signed endpoint", func(t *testing.T) {
		t.Setenv("RELKIT_RELEASE_VIA_CI", "1")
		t.Setenv("RELKIT_AGENT_TRANSPORT_URL", "   ")
		got, err := vNextAgentTransportURL(signed, true)
		if err != nil {
			t.Fatal(err)
		}
		if got != signed {
			t.Fatalf("endpoint=%q, want signed endpoint %q", got, signed)
		}
	})

	for name, override := range map[string]string{
		"relative path":       "/v1/publish",
		"missing scheme":      "publish.devcloud.woa.com",
		"userinfo":            "http://token@127.0.0.1:18787/",
		"userinfo with colon": "http://user:pass@publish.devcloud.woa.com/",
		"query":               "http://127.0.0.1:18787/?target=remote",
		"fragment":            "http://127.0.0.1:18787/#frag",
		"non HTTP":            "ssh://127.0.0.1:18787/",
		"file scheme":         "file:///etc/passwd",
	} {
		t.Run("reject "+name, func(t *testing.T) {
			t.Setenv("RELKIT_RELEASE_VIA_CI", "1")
			t.Setenv("RELKIT_AGENT_TRANSPORT_URL", override)
			if _, err := vNextAgentTransportURL(signed, true); err == nil {
				t.Fatalf("override %q unexpectedly accepted", override)
			}
		})
	}

	t.Run("non CI execute fails closed", func(t *testing.T) {
		t.Setenv("RELKIT_RELEASE_VIA_CI", "")
		t.Setenv("RELKIT_AGENT_TRANSPORT_URL", "http://publish.devcloud.woa.com/")
		_, err := vNextAgentTransportURL(signed, true)
		if err == nil || !strings.Contains(err.Error(), "RELKIT_RELEASE_VIA_CI=1") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("non CI execute fails closed for loopback too", func(t *testing.T) {
		t.Setenv("RELKIT_RELEASE_VIA_CI", "0")
		t.Setenv("RELKIT_AGENT_TRANSPORT_URL", "http://127.0.0.1:18787/")
		if _, err := vNextAgentTransportURL(signed, true); err == nil {
			t.Fatal("expected the CI gate to reject a non-CI execute override")
		}
	})
}
