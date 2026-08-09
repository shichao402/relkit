package backends

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/httpx"
)

type staticHTTPBackend struct {
	*pathStyleBackend
	stageDir string
	writable bool
	timeout  time.Duration
}

func newStaticHTTPBackend(name string, cfg map[string]any, root string) (Backend, error) {
	base, err := newPathStyleBackend(name, "static-http", cfg)
	if err != nil {
		return nil, err
	}

	stageDir := optionalString(cfg, "stageDir")
	writable := stageDir != ""
	if writable && !filepath.IsAbs(stageDir) {
		stageDir = filepath.Join(root, stageDir)
	}
	if writable {
		stageDir, err = filepath.Abs(stageDir)
		if err != nil {
			return nil, err
		}
	}

	return &staticHTTPBackend{
		pathStyleBackend: base,
		stageDir:         stageDir,
		writable:         writable,
		timeout:          optionalDurationSeconds(cfg, "timeoutSeconds", httpx.DefaultTimeout),
	}, nil
}

func (b *staticHTTPBackend) Describe() string {
	if b.writable {
		return fmt.Sprintf("%s (static-http -> %s, serving %s)", b.Name(), b.stageDir, b.baseURL)
	}
	return fmt.Sprintf("%s (static-http, read-only, %s)", b.Name(), b.baseURL)
}

func (b *staticHTTPBackend) URLsAreLive() bool {
	return true
}

func (b *staticHTTPBackend) Writable() bool {
	return b.writable
}

func (b *staticHTTPBackend) PutArtifact(localPath string, key string) ([]string, error) {
	if err := b.requireWritable(); err != nil {
		return nil, err
	}
	if err := b.copyFile(b.stageDir, key, localPath); err != nil {
		return nil, err
	}
	return []string{*b.URLFor(key)}, nil
}

func (b *staticHTTPBackend) PutImmutable(data []byte, key string) ([]string, error) {
	if err := b.requireWritable(); err != nil {
		return nil, err
	}
	if err := b.writeFile(b.stageDir, key, data); err != nil {
		return nil, err
	}
	return []string{*b.URLFor(key)}, nil
}

func (b *staticHTTPBackend) PutPointer(data []byte, key string) ([]string, error) {
	if err := b.requireWritable(); err != nil {
		return nil, err
	}
	if err := b.writeFile(b.stageDir, key, data); err != nil {
		return nil, err
	}
	return []string{*b.URLFor(key)}, nil
}

func (b *staticHTTPBackend) Get(key string) ([]byte, error) {
	return httpx.Get(*b.URLFor(key), b.timeout, strings.HasPrefix(key, "index/") || strings.HasPrefix(key, "fallback/") || strings.HasPrefix(key, "directory/"))
}

func (b *staticHTTPBackend) Probe(rawURL string) (bool, *int64, string) {
	return httpx.Probe(rawURL, b.timeout)
}

func (b *staticHTTPBackend) requireWritable() error {
	if b.writable {
		return nil
	}
	return Error{Message: fmt.Sprintf("backend %q has no 'stageDir', so it is read-only. Add one to publish through it (files are written there for the repository CI, rsync job or upload step to pick up), or use it only with 'relkit verify'.", b.Name())}
}
