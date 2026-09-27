// Package directory publishes the signed bootstrap UpdateDirectory (SPEC §16).
package directory

import (
	"crypto/ed25519"
	"fmt"
	"net/url"
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

type ServiceInput struct {
	ID          string
	Priority    int32
	IndexURL    string
	FallbackURL string
	Channel     string
}

type Options struct {
	Services []ServiceInput
	To       []string
	DryRun   bool
}

// Set replaces the product directory document: read current directory_sequence
// (if any), increment, sign once, and PutPointer the same bytes to every target.
func Set(cfg *config.Config, opts Options, printer Printer) (*model.UpdateDirectory, error) {
	if printer == nil {
		printer = func(string) {}
	}
	if cfg.Product == "" {
		return nil, Error{Message: "product is required"}
	}

	services := append([]ServiceInput(nil), opts.Services...)
	if len(services) == 0 {
		services = servicesFromConfig(cfg)
	}
	if len(services) == 0 {
		return nil, Error{Message: "provide at least one --service, or configure directory.services in relkit.json"}
	}
	if err := validateServices(services); err != nil {
		return nil, err
	}

	targetNames := append([]string(nil), opts.To...)
	if len(targetNames) == 0 && cfg.Directory != nil && len(cfg.Directory.PublishTo) > 0 {
		targetNames = append([]string(nil), cfg.Directory.PublishTo...)
	}
	if len(targetNames) == 0 {
		targetNames = append([]string(nil), cfg.PublishTo...)
	}
	if len(targetNames) == 0 {
		return nil, Error{Message: "no target backends; set directory.publishTo / publishTo or pass --to"}
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
	key := model.DirectoryKey(cfg.Product)
	baseSequence, err := readBaseSequence(opened, key, cfg.Product, trusted, printer)
	if err != nil {
		return nil, err
	}

	docServices := make([]*model.DirectoryService, 0, len(services))
	for _, in := range services {
		docServices = append(docServices, &model.DirectoryService{
			Id:          in.ID,
			Priority:    in.Priority,
			IndexUrl:    in.IndexURL,
			FallbackUrl: in.FallbackURL,
			Channel:     in.Channel,
		})
	}

	doc := &model.UpdateDirectory{
		Schema:            model.SchemaDirectory,
		Product:           cfg.Product,
		DirectorySequence: int64(baseSequence + 1),
		UpdatedAt:         model.UTCNow(),
		Services:          docServices,
	}

	printer(fmt.Sprintf("directory %s sequence %d (%d service(s))", cfg.Product, doc.DirectorySequence, len(doc.Services)))
	for _, service := range doc.Services {
		channel := service.Channel
		if channel == "" {
			channel = "*"
		}
		printer(fmt.Sprintf("  %-16s priority=%d channel=%s index=%s", service.Id, service.Priority, channel, service.IndexUrl))
	}

	if opts.DryRun {
		printer("dry run: nothing uploaded")
		printer("  would write " + key)
		return doc, nil
	}

	env, err := envelope.SealDirectory(doc, signers)
	if err != nil {
		return nil, err
	}
	envelopeBytes, err := rupv2.MarshalEnvelope(env)
	if err != nil {
		return nil, err
	}
	if err := checkClientsCanVerifyDirectory(cfg, env); err != nil {
		return nil, err
	}

	for _, backend := range opened {
		if _, err := backend.PutPointer(envelopeBytes, key); err != nil {
			return nil, Error{Message: fmt.Sprintf("backend %q failed to write %s: %v", backend.Name(), key, err)}
		}
		printer(fmt.Sprintf("  %-12s %s", backend.Name(), key))
	}
	printer(fmt.Sprintf("published directory for %s at sequence %d", cfg.Product, doc.DirectorySequence))
	return doc, nil
}

func servicesFromConfig(cfg *config.Config) []ServiceInput {
	if cfg.Directory == nil {
		return nil
	}
	out := make([]ServiceInput, 0, len(cfg.Directory.Services))
	for _, service := range cfg.Directory.Services {
		out = append(out, ServiceInput{
			ID:          service.ID,
			Priority:    service.Priority,
			IndexURL:    service.IndexURL,
			FallbackURL: service.FallbackURL,
			Channel:     service.Channel,
		})
	}
	return out
}

func validateServices(services []ServiceInput) error {
	ids := map[string]struct{}{}
	for i, service := range services {
		if strings.TrimSpace(service.ID) == "" {
			return Error{Message: fmt.Sprintf("service %d: id is required", i+1)}
		}
		if _, exists := ids[service.ID]; exists {
			return Error{Message: fmt.Sprintf("service id %q is duplicated", service.ID)}
		}
		ids[service.ID] = struct{}{}
		if err := requireAbsoluteURL(service.IndexURL, fmt.Sprintf("service %s index-url", service.ID)); err != nil {
			return err
		}
		if service.FallbackURL != "" {
			if err := requireAbsoluteURL(service.FallbackURL, fmt.Sprintf("service %s fallback-url", service.ID)); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireAbsoluteURL(raw, what string) error {
	if strings.TrimSpace(raw) == "" {
		return Error{Message: what + " is required"}
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Error{Message: fmt.Sprintf("%s must be an absolute http(s) URL, got %q", what, raw)}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Error{Message: fmt.Sprintf("%s must be http(s), got %q", what, raw)}
	}
	return nil
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

func checkClientsCanVerifyDirectory(cfg *config.Config, env *model.Envelope) error {
	trusted, err := cfg.TrustedPublicKeys()
	if err != nil || len(trusted) == 0 {
		return nil
	}
	if _, err := envelope.OpenDirectoryEnvelope(env, trusted); err != nil {
		return Error{Message: fmt.Sprintf("the directory was signed, but the configured public keys reject it: %v", err)}
	}
	return nil
}

func readBaseSequence(opened []backends.Backend, key, product string, trusted map[string]ed25519.PublicKey, printer Printer) (int, error) {
	base := 0
	for _, backend := range opened {
		raw, err := backend.Get(key)
		if err != nil {
			return 0, Error{Message: fmt.Sprintf("could not read directory from backend %q: %v", backend.Name(), err)}
		}
		if raw == nil {
			printer(fmt.Sprintf("  %-12s no existing directory", backend.Name()))
			continue
		}
		env, err := rupv2.UnmarshalEnvelope(raw)
		if err != nil {
			return 0, Error{Message: fmt.Sprintf("directory on backend %q is not valid protobuf: %v", backend.Name(), err)}
		}
		doc, err := envelope.OpenDirectoryEnvelope(env, trusted)
		if err != nil {
			return 0, Error{Message: fmt.Sprintf("directory on backend %q failed verification: %v", backend.Name(), err)}
		}
		if doc.Product != product {
			return 0, Error{Message: fmt.Sprintf("directory on backend %q is for product %q, expected %q", backend.Name(), doc.Product, product)}
		}
		printer(fmt.Sprintf("  %-12s sequence %d, %d service(s)", backend.Name(), doc.DirectorySequence, len(doc.Services)))
		if int(doc.DirectorySequence) > base {
			base = int(doc.DirectorySequence)
		}
	}
	return base, nil
}
