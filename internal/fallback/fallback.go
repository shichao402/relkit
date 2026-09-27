// Package fallback publishes the signed emergency notice (SPEC §12.6).
package fallback

import (
	"crypto/ed25519"
	"fmt"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/backends"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/model"
)

type Error struct {
	Message string
}

func (e Error) Error() string {
	return e.Message
}

type Printer func(string)

type RuleInput struct {
	MinCode   int64
	MaxCode   int64
	ManualURL string
	Message   string
	Mandatory bool
	Selectors map[string]string
}

type Options struct {
	Clear  bool
	Rules  []RuleInput
	To     []string
	DryRun bool
}

// Set replaces the product fallback document: read current sequence (if any),
// increment, sign, and PutPointer fallback/<product>.pb.
func Set(cfg *config.Config, opts Options, printer Printer) (*model.FallbackDocument, error) {
	if printer == nil {
		printer = func(string) {}
	}
	if cfg.Product == "" {
		return nil, Error{Message: "product is required"}
	}
	if !opts.Clear && len(opts.Rules) == 0 {
		return nil, Error{Message: "provide at least one rule, or pass --clear"}
	}
	if opts.Clear && len(opts.Rules) > 0 {
		return nil, Error{Message: "--clear cannot be combined with rule flags"}
	}

	for i, rule := range opts.Rules {
		if rule.MaxCode < 1 {
			return nil, Error{Message: fmt.Sprintf("rule %d: --max-code must be >= 1", i+1)}
		}
		if rule.MinCode < 0 {
			return nil, Error{Message: fmt.Sprintf("rule %d: --min-code must be >= 0", i+1)}
		}
		if rule.MinCode > rule.MaxCode {
			return nil, Error{Message: fmt.Sprintf("rule %d: --min-code (%d) > --max-code (%d)", i+1, rule.MinCode, rule.MaxCode)}
		}
		if strings.TrimSpace(rule.ManualURL) == "" {
			return nil, Error{Message: fmt.Sprintf("rule %d: --url is required", i+1)}
		}
		if !strings.HasPrefix(rule.ManualURL, "http://") && !strings.HasPrefix(rule.ManualURL, "https://") {
			return nil, Error{Message: fmt.Sprintf("rule %d: --url must be an absolute http(s) URL", i+1)}
		}
	}

	targetNames := append([]string(nil), opts.To...)
	if len(targetNames) == 0 {
		targetNames = append([]string(nil), cfg.PublishTo...)
	}
	if len(targetNames) == 0 {
		return nil, Error{Message: "no target backends; set publishTo or pass --to"}
	}

	opened := make([]backends.Backend, 0, len(targetNames))
	for _, name := range targetNames {
		backend, err := backends.Create(name, cfg, cfg.Root)
		if err != nil {
			return nil, err
		}
		if !backend.Writable() {
			return nil, Error{Message: fmt.Sprintf("backend %q is read-only", name)}
		}
		opened = append(opened, backend)
	}

	var signers []envelope.Signer
	var err error
	if !opts.DryRun {
		signers, err = cfg.LoadSigners()
		if err != nil {
			return nil, err
		}
	}

	trusted := trustedKeys(cfg, signers)
	key := model.FallbackKey(cfg.Product)
	baseSequence, err := readBaseSequence(opened, key, cfg.Product, trusted, printer)
	if err != nil {
		return nil, err
	}

	rules := make([]*model.FallbackRule, 0, len(opts.Rules))
	for _, in := range opts.Rules {
		rules = append(rules, &model.FallbackRule{
			MinCode:   in.MinCode,
			MaxCode:   in.MaxCode,
			ManualUrl: in.ManualURL,
			Message:   in.Message,
			Mandatory: in.Mandatory,
			Selectors: model.SelectorsFromMap(in.Selectors),
		})
	}

	doc := &model.FallbackDocument{
		Schema:      model.SchemaFallback,
		Product:     cfg.Product,
		Sequence:    int64(baseSequence + 1),
		GeneratedAt: model.UTCNow(),
		Rules:       rules,
	}

	printer(fmt.Sprintf("fallback %s sequence %d (%d rule(s))", cfg.Product, doc.Sequence, len(doc.Rules)))
	for i, rule := range doc.Rules {
		printer(fmt.Sprintf("  rule %d: codes [%d..%d] mandatory=%v url=%s", i+1, rule.MinCode, rule.MaxCode, rule.Mandatory, rule.ManualUrl))
	}

	if opts.DryRun {
		printer("dry run: nothing uploaded")
		printer("  would write " + key)
		return doc, nil
	}

	env, err := envelope.SealFallback(doc, signers)
	if err != nil {
		return nil, err
	}
	envelopeBytes, err := rupv2.MarshalEnvelope(env)
	if err != nil {
		return nil, err
	}
	if err := checkClientsCanVerifyFallback(cfg, env); err != nil {
		return nil, err
	}

	for _, backend := range opened {
		if _, err := backend.PutPointer(envelopeBytes, key); err != nil {
			return nil, Error{Message: fmt.Sprintf("backend %q failed to write %s: %v", backend.Name(), key, err)}
		}
		printer(fmt.Sprintf("  %-12s %s", backend.Name(), key))
	}
	printer(fmt.Sprintf("published fallback for %s at sequence %d", cfg.Product, doc.Sequence))
	return doc, nil
}

func trustedKeys(cfg *config.Config, signers []envelope.Signer) map[string]ed25519.PublicKey {
	trusted := map[string]ed25519.PublicKey{}
	if configured, err := cfg.TrustedPublicKeys(); err == nil {
		for keyID, publicKey := range configured {
			trusted[keyID] = publicKey
		}
	}
	for _, signer := range signers {
		trusted[signer.KeyID] = signer.PublicKey()
	}
	return trusted
}

func checkClientsCanVerifyFallback(cfg *config.Config, env *model.Envelope) error {
	trusted, err := cfg.TrustedPublicKeys()
	if err != nil || len(trusted) == 0 {
		return nil
	}
	if _, err := envelope.OpenFallbackEnvelope(env, trusted); err != nil {
		return Error{Message: fmt.Sprintf("the fallback was signed, but the configured public keys reject it: %v", err)}
	}
	return nil
}

func readBaseSequence(opened []backends.Backend, key, product string, trusted map[string]ed25519.PublicKey, printer Printer) (int, error) {
	base := 0
	for _, backend := range opened {
		raw, err := backend.Get(key)
		if err != nil {
			return 0, Error{Message: fmt.Sprintf("could not read fallback from backend %q: %v", backend.Name(), err)}
		}
		if raw == nil {
			printer(fmt.Sprintf("  %-12s no existing fallback", backend.Name()))
			continue
		}
		env, err := rupv2.UnmarshalEnvelope(raw)
		if err != nil {
			return 0, Error{Message: fmt.Sprintf("fallback on backend %q is not valid protobuf: %v", backend.Name(), err)}
		}
		doc, err := envelope.OpenFallbackEnvelope(env, trusted)
		if err != nil {
			return 0, Error{Message: fmt.Sprintf("fallback on backend %q failed verification: %v", backend.Name(), err)}
		}
		if doc.Product != product {
			return 0, Error{Message: fmt.Sprintf("fallback on backend %q is for product %q, expected %q", backend.Name(), doc.Product, product)}
		}
		printer(fmt.Sprintf("  %-12s sequence %d, %d rule(s)", backend.Name(), doc.Sequence, len(doc.Rules)))
		if int(doc.Sequence) > base {
			base = int(doc.Sequence)
		}
	}
	return base, nil
}
