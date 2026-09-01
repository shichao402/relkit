package envelope_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"cnb.cool/shichao402/relkit/internal/envelope"
	"cnb.cool/shichao402/relkit/internal/keys"
	"cnb.cool/shichao402/relkit/internal/model"
	"cnb.cool/shichao402/relkit/internal/testutil"
)

type signatureKeysFixture struct {
	Keys []struct {
		KeyID             string `json:"keyId"`
		PublicKeyBase64   string `json:"publicKeyBase64"`
		PrivateSeedBase64 string `json:"privateSeedBase64"`
	} `json:"keys"`
}

type envelopeFixture struct {
	Name          string         `json:"name"`
	TrustedKeys   []string       `json:"trustedKeys"`
	ExpectProduct string         `json:"expectProduct"`
	ExpectChannel string         `json:"expectChannel"`
	Cases         []envelopeCase `json:"cases"`
}

type envelopeCase struct {
	Name           string         `json:"name"`
	Envelope       model.Envelope `json:"envelope"`
	ExpectAccepted bool           `json:"expectAccepted"`
}

type antiRollbackFixture struct {
	Cases []struct {
		LastSeenSequence *int `json:"lastSeenSequence"`
		IndexSequence    int  `json:"indexSequence"`
		ExpectAccepted   bool `json:"expectAccepted"`
	} `json:"cases"`
}

func TestEnvelopeConformance(t *testing.T) {
	root := filepath.Join(testutil.ConformanceRoot(t), "signature")
	trusted := loadTrustedKeys(t, root, []string{"k1", "k2"})

	fixture := loadEnvelopeCasesFixture(t, filepath.Join(root, "envelope.json"))
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			got := envelope.AcceptEnvelope(&tc.Envelope, trusted, fixture.ExpectProduct, fixture.ExpectChannel)
			if got != tc.ExpectAccepted {
				t.Fatalf("AcceptEnvelope mismatch: got %v want %v", got, tc.ExpectAccepted)
			}
		})
	}
}

func TestAntiRollbackConformance(t *testing.T) {
	root := filepath.Join(testutil.ConformanceRoot(t), "signature")
	var fixture antiRollbackFixture
	loadEnvelopeFixture(t, filepath.Join(root, "anti-rollback.json"), &fixture)
	for _, tc := range fixture.Cases {
		got := envelope.AcceptSequence(tc.LastSeenSequence, tc.IndexSequence)
		if got != tc.ExpectAccepted {
			t.Fatalf("lastSeen=%v index=%d mismatch: got %v want %v", tc.LastSeenSequence, tc.IndexSequence, got, tc.ExpectAccepted)
		}
	}
}

func loadTrustedKeys(t *testing.T, root string, trustedIDs []string) map[string]ed25519.PublicKey {
	t.Helper()
	var fixture signatureKeysFixture
	loadEnvelopeFixture(t, filepath.Join(root, "keys.json"), &fixture)
	wanted := make(map[string]struct{}, len(trustedIDs))
	for _, keyID := range trustedIDs {
		wanted[keyID] = struct{}{}
	}
	trusted := map[string]ed25519.PublicKey{}
	for _, entry := range fixture.Keys {
		if _, ok := wanted[entry.KeyID]; !ok {
			continue
		}
		key, err := keys.DecodePublicKey(entry.PublicKeyBase64, entry.KeyID)
		if err != nil {
			t.Fatal(err)
		}
		trusted[entry.KeyID] = key
	}
	return trusted
}

func loadEnvelopeFixture(t *testing.T, path string, dest any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatal(err)
	}
}

func loadEnvelopeCasesFixture(t *testing.T, path string) envelopeFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var raw struct {
		Name          string   `json:"name"`
		TrustedKeys   []string `json:"trustedKeys"`
		ExpectProduct string   `json:"expectProduct"`
		ExpectChannel string   `json:"expectChannel"`
		Cases         []struct {
			Name     string `json:"name"`
			Envelope struct {
				Schema     string `json:"schema"`
				Payload    string `json:"payload"`
				Signatures []struct {
					KeyID string `json:"keyId"`
					Alg   string `json:"alg"`
					Sig   string `json:"sig"`
				} `json:"signatures"`
			} `json:"envelope"`
			ExpectAccepted bool `json:"expectAccepted"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}

	fixture := envelopeFixture{
		Name:          raw.Name,
		TrustedKeys:   raw.TrustedKeys,
		ExpectProduct: raw.ExpectProduct,
		ExpectChannel: raw.ExpectChannel,
		Cases:         make([]envelopeCase, 0, len(raw.Cases)),
	}
	for _, tc := range raw.Cases {
		payload, err := base64.StdEncoding.DecodeString(tc.Envelope.Payload)
		if err != nil {
			t.Fatalf("%s: payload: %v", tc.Name, err)
		}
		env := model.Envelope{
			Schema:  tc.Envelope.Schema,
			Payload: payload,
		}
		for _, entry := range tc.Envelope.Signatures {
			sig, err := base64.StdEncoding.DecodeString(entry.Sig)
			if err != nil {
				t.Fatalf("%s: sig %s: %v", tc.Name, entry.KeyID, err)
			}
			env.Signatures = append(env.Signatures, &model.Signature{
				KeyId: entry.KeyID,
				Alg:   entry.Alg,
				Sig:   sig,
			})
		}
		fixture.Cases = append(fixture.Cases, envelopeCase{
			Name:           tc.Name,
			Envelope:       env,
			ExpectAccepted: tc.ExpectAccepted,
		})
	}
	return fixture
}
