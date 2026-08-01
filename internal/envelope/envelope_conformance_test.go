package envelope_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/testutil"
	"google.golang.org/protobuf/proto"
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

	seeds := loadSigningSeeds(t, filepath.Dir(path))
	var baseIndex *model.IndexDocument
	for _, tc := range raw.Cases {
		index, err := decodeLegacyPayloadIndex(tc.Envelope.Payload)
		if err == nil {
			baseIndex = index
			break
		}
	}
	if baseIndex == nil {
		t.Fatal("could not decode any legacy payload from envelope fixtures")
	}

	fixture := envelopeFixture{
		Name:          raw.Name,
		TrustedKeys:   raw.TrustedKeys,
		ExpectProduct: raw.ExpectProduct,
		ExpectChannel: raw.ExpectChannel,
		Cases:         make([]envelopeCase, 0, len(raw.Cases)),
	}
	for _, tc := range raw.Cases {
		index, err := decodeLegacyPayloadIndex(tc.Envelope.Payload)
		if err != nil {
			if tc.Name != "tampered-payload" {
				t.Fatal(err)
			}
			index = proto.Clone(baseIndex).(*model.IndexDocument)
		}
		env, err := buildProtoEnvelopeCase(tc.Name, index, tc.Envelope, seeds)
		if err != nil {
			t.Fatal(err)
		}
		fixture.Cases = append(fixture.Cases, envelopeCase{
			Name:           tc.Name,
			Envelope:       *env,
			ExpectAccepted: tc.ExpectAccepted,
		})
	}
	return fixture
}

func loadSigningSeeds(t *testing.T, root string) map[string][]byte {
	t.Helper()
	var fixture signatureKeysFixture
	loadEnvelopeFixture(t, filepath.Join(root, "keys.json"), &fixture)
	seeds := make(map[string][]byte, len(fixture.Keys))
	for _, entry := range fixture.Keys {
		if entry.PrivateSeedBase64 == "" {
			continue
		}
		seed, err := keys.DecodeSeed(entry.PrivateSeedBase64, entry.KeyID)
		if err != nil {
			t.Fatal(err)
		}
		seeds[entry.KeyID] = seed
	}
	return seeds
}

type legacyIndexDocument struct {
	Product      string              `json:"product"`
	Channel      string              `json:"channel"`
	Sequence     int64               `json:"sequence"`
	GeneratedAt  string              `json:"generatedAt"`
	MinSupported *int64              `json:"minSupported"`
	ExpiresAt    string              `json:"expiresAt"`
	Versions     []legacyVersionNode `json:"versions"`
}

type legacyVersionNode struct {
	Version    string            `json:"version"`
	Code       int64             `json:"code"`
	MinFrom    int64             `json:"minFrom"`
	ReleasedAt string            `json:"releasedAt"`
	Yanked     bool              `json:"yanked"`
	Notes      string            `json:"notes"`
	NotesURL   string            `json:"notesUrl"`
	Manifest   legacyManifestRef `json:"manifest"`
}

type legacyManifestRef struct {
	SHA256 string   `json:"sha256"`
	Size   int64    `json:"size"`
	URLs   []string `json:"urls"`
}

func decodeLegacyPayloadIndex(encoded string) (*model.IndexDocument, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	var legacy legacyIndexDocument
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, err
	}
	index := &model.IndexDocument{
		Schema:      model.SchemaIndex,
		Product:     legacy.Product,
		Channel:     legacy.Channel,
		Sequence:    legacy.Sequence,
		GeneratedAt: legacy.GeneratedAt,
		ExpiresAt:   legacy.ExpiresAt,
		Versions:    make([]*model.VersionNode, 0, len(legacy.Versions)),
	}
	if legacy.MinSupported != nil {
		index.MinSupported = *legacy.MinSupported
		index.HasMinSupported = true
	}
	for _, version := range legacy.Versions {
		index.Versions = append(index.Versions, &model.VersionNode{
			Version:    version.Version,
			Code:       version.Code,
			MinFrom:    version.MinFrom,
			ReleasedAt: version.ReleasedAt,
			Yanked:     version.Yanked,
			Notes:      version.Notes,
			NotesUrl:   version.NotesURL,
			Manifest: &model.ManifestRef{
				Sha256: version.Manifest.SHA256,
				Size:   version.Manifest.Size,
				Urls:   append([]string(nil), version.Manifest.URLs...),
			},
		})
	}
	return index, nil
}

func buildProtoEnvelopeCase(name string, index *model.IndexDocument, raw struct {
	Schema     string `json:"schema"`
	Payload    string `json:"payload"`
	Signatures []struct {
		KeyID string `json:"keyId"`
		Alg   string `json:"alg"`
		Sig   string `json:"sig"`
	} `json:"signatures"`
}, seeds map[string][]byte) (*model.Envelope, error) {
	payload, err := rupv2.MarshalIndex(index)
	if err != nil {
		return nil, err
	}
	env := &model.Envelope{
		Schema:  model.SchemaEnvelope,
		Payload: payload,
	}
	if name == "wrong-envelope-schema" {
		env.Schema = "rup.envelope/3"
	}
	for _, entry := range raw.Signatures {
		sigBytes, err := buildSignatureBytes(name, index, payload, entry.KeyID, entry.Alg, entry.Sig, seeds)
		if err != nil {
			return nil, err
		}
		env.Signatures = append(env.Signatures, &model.Signature{
			KeyId: entry.KeyID,
			Alg:   entry.Alg,
			Sig:   sigBytes,
		})
	}
	if name == "tampered-payload" && len(env.Payload) > 0 {
		env.Payload[len(env.Payload)-1] ^= 0x01
	}
	return env, nil
}

func buildSignatureBytes(name string, index *model.IndexDocument, payload []byte, keyID, alg, rawSig string, seeds map[string][]byte) ([]byte, error) {
	if alg != "ed25519" {
		return base64.StdEncoding.DecodeString(rawSig)
	}
	seed := seeds[keyID]
	if len(seed) == 0 {
		return base64.StdEncoding.DecodeString(rawSig)
	}
	message := payload
	if name == "cross-payload-replay" {
		other := proto.Clone(index).(*model.IndexDocument)
		other.Sequence++
		alternate, err := rupv2.MarshalIndex(other)
		if err != nil {
			return nil, err
		}
		message = alternate
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(seed), message)
	if name == "bad-signature" && keyID == "k1" && len(signature) > 0 {
		signature[len(signature)-1] ^= 0x01
	}
	return signature, nil
}
