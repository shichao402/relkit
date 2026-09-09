package backends

import (
	"fmt"
	"strings"
	"time"

	"go.firoyang.com/relkit/internal/httpx"
)

type staticHTTPBackend struct {
	*pathStyleBackend
	timeout time.Duration
}

func newStaticHTTPBackend(name string, cfg map[string]any, root string) (Backend, error) {
	_ = root
	base, err := newPathStyleBackend(name, "static-http", cfg)
	if err != nil {
		return nil, err
	}
	if optionalString(cfg, "stageDir") != "" {
		return nil, Error{Message: fmt.Sprintf("backend %q: static-http is read-only and no longer accepts stageDir; use relkit-compatible for self-hosted publishing", name)}
	}

	return &staticHTTPBackend{
		pathStyleBackend: base,
		timeout:          optionalDurationSeconds(cfg, "timeoutSeconds", httpx.DefaultTimeout),
	}, nil
}

func (b *staticHTTPBackend) Describe() string {
	return fmt.Sprintf("%s (static-http, read-only, %s)", b.Name(), b.baseURL)
}

func (b *staticHTTPBackend) URLsAreLive() bool {
	return true
}

func (b *staticHTTPBackend) Writable() bool {
	return false
}

func (b *staticHTTPBackend) PutArtifact(localPath string, key string) ([]string, error) {
	return nil, b.readOnlyError()
}

func (b *staticHTTPBackend) PutImmutable(data []byte, key string) ([]string, error) {
	return nil, b.readOnlyError()
}

func (b *staticHTTPBackend) PutPointer(data []byte, key string) ([]string, error) {
	return nil, b.readOnlyError()
}

func (b *staticHTTPBackend) Get(key string) ([]byte, error) {
	return httpx.Get(*b.URLFor(key), b.timeout, strings.HasPrefix(key, "index/") || strings.HasPrefix(key, "fallback/") || strings.HasPrefix(key, "directory/"))
}

func (b *staticHTTPBackend) Probe(rawURL string) (bool, *int64, string) {
	return httpx.Probe(rawURL, b.timeout)
}

func (b *staticHTTPBackend) readOnlyError() error {
	return Error{Message: fmt.Sprintf("backend %q is read-only; use relkit-compatible or s3-compatible to publish", b.Name())}
}
