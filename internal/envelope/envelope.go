package envelope

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"github.com/shichao402/relkit/internal/jsonio"
	"github.com/shichao402/relkit/internal/model"
)

const (
	EnvelopeSchema = model.SchemaEnvelope
	IndexSchema    = model.SchemaIndex
)

type Error struct {
	Message string
}

func (e Error) Error() string {
	return e.Message
}

type Signer struct {
	KeyID string
	Seed  []byte
}

func (s Signer) PublicKey() ed25519.PublicKey {
	privateKey := ed25519.NewKeyFromSeed(s.Seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	cloned := make(ed25519.PublicKey, len(publicKey))
	copy(cloned, publicKey)
	return cloned
}

func Seal(index *model.IndexDocument, signers []Signer) (*model.Envelope, error) {
	if len(signers) == 0 {
		return nil, Error{Message: "at least one signer is required"}
	}

	payload, err := jsonio.MarshalCompact(index)
	if err != nil {
		return nil, err
	}

	signatures := make([]model.Signature, 0, len(signers))
	for _, signer := range signers {
		privateKey := ed25519.NewKeyFromSeed(signer.Seed)
		signature := ed25519.Sign(privateKey, payload)
		signatures = append(signatures, model.Signature{
			KeyID: signer.KeyID,
			Alg:   "ed25519",
			Sig:   base64.StdEncoding.EncodeToString(signature),
		})
	}

	return &model.Envelope{
		Schema:     EnvelopeSchema,
		Payload:    base64.StdEncoding.EncodeToString(payload),
		Signatures: signatures,
	}, nil
}

func VerifiedPayload(env *model.Envelope, trustedKeys map[string]ed25519.PublicKey) []byte {
	if env == nil || env.Schema != EnvelopeSchema {
		return nil
	}

	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil
	}

	for _, entry := range env.Signatures {
		if entry.Alg != "ed25519" {
			continue
		}
		publicKey, ok := trustedKeys[entry.KeyID]
		if !ok {
			continue
		}
		signature, err := base64.StdEncoding.DecodeString(entry.Sig)
		if err != nil {
			continue
		}
		if ed25519.Verify(publicKey, payload, signature) {
			return payload
		}
	}
	return nil
}

func AcceptEnvelope(env *model.Envelope, trustedKeys map[string]ed25519.PublicKey, expectProduct, expectChannel string) bool {
	payload := VerifiedPayload(env, trustedKeys)
	if payload == nil {
		return false
	}

	var index model.IndexDocument
	if err := jsonio.LoadBytes(payload, &index); err != nil {
		return false
	}
	if index.Schema != IndexSchema {
		return false
	}
	if index.Product != expectProduct {
		return false
	}
	return index.Channel == expectChannel
}

func AcceptSequence(lastSeenSequence *int, indexSequence int) bool {
	if lastSeenSequence == nil {
		return true
	}
	return indexSequence >= *lastSeenSequence
}

func OpenEnvelope(env *model.Envelope, trustedKeys map[string]ed25519.PublicKey) (*model.IndexDocument, error) {
	if env == nil {
		return nil, Error{Message: "envelope is not a JSON object"}
	}
	if env.Schema != EnvelopeSchema {
		return nil, Error{Message: fmt.Sprintf("unexpected envelope schema: %q", env.Schema)}
	}
	if len(env.Signatures) == 0 {
		return nil, Error{Message: "envelope carries no signatures"}
	}

	payload := VerifiedPayload(env, trustedKeys)
	if payload == nil {
		keyIDs := make([]string, 0, len(env.Signatures))
		for _, entry := range env.Signatures {
			keyIDs = append(keyIDs, entry.KeyID)
		}
		return nil, Error{Message: fmt.Sprintf("no trusted key verified this envelope (signed by: %s; trusted: %s)", joinOrNone(keyIDs), joinSortedOrNone(trustedKeys))}
	}

	var index model.IndexDocument
	if err := jsonio.LoadBytes(payload, &index); err != nil {
		return nil, err
	}
	if index.Schema != IndexSchema {
		return nil, Error{Message: fmt.Sprintf("unexpected index schema: %q", index.Schema)}
	}
	return &index, nil
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	sort.Strings(values)
	return strings.Join(values, ", ")
}

func joinSortedOrNone(values map[string]ed25519.PublicKey) string {
	if len(values) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
