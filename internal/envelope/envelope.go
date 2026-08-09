package envelope

import (
	"crypto/ed25519"
	"fmt"
	"sort"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/model"
)

const (
	EnvelopeSchema  = model.SchemaEnvelope
	IndexSchema     = model.SchemaIndex
	FallbackSchema  = model.SchemaFallback
	DirectorySchema = model.SchemaDirectory
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

	payload, err := rupv2.MarshalIndex(index)
	if err != nil {
		return nil, err
	}

	return sealPayload(payload, signers)
}

func SealFallback(doc *model.FallbackDocument, signers []Signer) (*model.Envelope, error) {
	if len(signers) == 0 {
		return nil, Error{Message: "at least one signer is required"}
	}

	payload, err := rupv2.MarshalFallback(doc)
	if err != nil {
		return nil, err
	}

	return sealPayload(payload, signers)
}

func SealDirectory(doc *model.UpdateDirectory, signers []Signer) (*model.Envelope, error) {
	if len(signers) == 0 {
		return nil, Error{Message: "at least one signer is required"}
	}

	payload, err := rupv2.MarshalDirectory(doc)
	if err != nil {
		return nil, err
	}

	return sealPayload(payload, signers)
}

func sealPayload(payload []byte, signers []Signer) (*model.Envelope, error) {
	signatures := make([]*model.Signature, 0, len(signers))
	for _, signer := range signers {
		privateKey := ed25519.NewKeyFromSeed(signer.Seed)
		signature := ed25519.Sign(privateKey, payload)
		signatures = append(signatures, &model.Signature{
			KeyId: signer.KeyID,
			Alg:   "ed25519",
			Sig:   signature,
		})
	}

	return &model.Envelope{
		Schema:     EnvelopeSchema,
		Payload:    payload,
		Signatures: signatures,
	}, nil
}

func VerifiedPayload(env *model.Envelope, trustedKeys map[string]ed25519.PublicKey) []byte {
	if env == nil || env.Schema != EnvelopeSchema {
		return nil
	}

	payload := env.Payload

	for _, entry := range env.Signatures {
		if entry == nil {
			continue
		}
		if entry.Alg != "ed25519" {
			continue
		}
		publicKey, ok := trustedKeys[entry.KeyId]
		if !ok {
			continue
		}
		if ed25519.Verify(publicKey, payload, entry.Sig) {
			return append([]byte(nil), payload...)
		}
	}
	return nil
}

func AcceptEnvelope(env *model.Envelope, trustedKeys map[string]ed25519.PublicKey, expectProduct, expectChannel string) bool {
	payload := VerifiedPayload(env, trustedKeys)
	if payload == nil {
		return false
	}

	index, err := rupv2.UnmarshalIndex(payload)
	if err != nil {
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
	payload, err := verifiedEnvelopePayload(env, trustedKeys)
	if err != nil {
		return nil, err
	}

	index, err := rupv2.UnmarshalIndex(payload)
	if err != nil {
		return nil, err
	}
	if index.Schema != IndexSchema {
		return nil, Error{Message: fmt.Sprintf("unexpected index schema: %q", index.Schema)}
	}
	return index, nil
}

func OpenFallbackEnvelope(env *model.Envelope, trustedKeys map[string]ed25519.PublicKey) (*model.FallbackDocument, error) {
	payload, err := verifiedEnvelopePayload(env, trustedKeys)
	if err != nil {
		return nil, err
	}

	doc, err := rupv2.UnmarshalFallback(payload)
	if err != nil {
		return nil, err
	}
	if doc.Schema != FallbackSchema {
		return nil, Error{Message: fmt.Sprintf("unexpected fallback schema: %q", doc.Schema)}
	}
	return doc, nil
}

func OpenDirectoryEnvelope(env *model.Envelope, trustedKeys map[string]ed25519.PublicKey) (*model.UpdateDirectory, error) {
	payload, err := verifiedEnvelopePayload(env, trustedKeys)
	if err != nil {
		return nil, err
	}

	doc, err := rupv2.UnmarshalDirectory(payload)
	if err != nil {
		return nil, err
	}
	if doc.Schema != DirectorySchema {
		return nil, Error{Message: fmt.Sprintf("unexpected directory schema: %q", doc.Schema)}
	}
	return doc, nil
}

func verifiedEnvelopePayload(env *model.Envelope, trustedKeys map[string]ed25519.PublicKey) ([]byte, error) {
	if env == nil {
		return nil, Error{Message: "envelope is not a protobuf message"}
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
			if entry == nil {
				continue
			}
			keyIDs = append(keyIDs, entry.KeyId)
		}
		return nil, Error{Message: fmt.Sprintf("no trusted key verified this envelope (signed by: %s; trusted: %s)", joinOrNone(keyIDs), joinSortedOrNone(trustedKeys))}
	}
	return payload, nil
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
