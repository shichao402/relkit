package main

import (
	"strings"
	"testing"
)

func TestVNextAgentTransportURLAllowsOnlyCILoopback(t *testing.T) {
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

	t.Run("CI may use IPv4 loopback tunnel", func(t *testing.T) {
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

	t.Run("CI may use IPv6 loopback tunnel", func(t *testing.T) {
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

	for name, override := range map[string]string{
		"remote host": "https://operator.example.invalid/",
		"localhost":   "http://localhost:18787/",
		"userinfo":    "http://token@127.0.0.1:18787/",
		"query":       "http://127.0.0.1:18787/?target=remote",
		"non HTTP":    "ssh://127.0.0.1:18787/",
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
		t.Setenv("RELKIT_AGENT_TRANSPORT_URL", "http://127.0.0.1:18787/")
		_, err := vNextAgentTransportURL(signed, true)
		if err == nil || !strings.Contains(err.Error(), "RELKIT_RELEASE_VIA_CI=1") {
			t.Fatalf("err=%v", err)
		}
	})
}
