package backends

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/httpx"
)

var contentTypes = map[string]string{
	".json": "application/json",
	".zip":  "application/zip",
	".gz":   "application/gzip",
	".tgz":  "application/gzip",
}

type httpPutBackend struct {
	*pathStyleBackend
	uploadURL string
	tokenEnv  string
	timeout   time.Duration
}

func newHTTPPutBackend(name string, cfg map[string]any, root string) (Backend, error) {
	base, err := newPathStyleBackend(name, "http-put", cfg)
	if err != nil {
		return nil, err
	}

	uploadURL := optionalString(cfg, "uploadUrl")
	if uploadURL == "" {
		uploadURL = base.baseURL
	}
	if !strings.HasPrefix(uploadURL, "http://") && !strings.HasPrefix(uploadURL, "https://") {
		return nil, Error{Message: fmt.Sprintf("uploadUrl of backend %q must be an absolute http(s) URL, got %q", name, uploadURL)}
	}
	if !strings.HasSuffix(uploadURL, "/") {
		uploadURL += "/"
	}

	tokenEnv, err := requiredString(cfg, "tokenEnv", name)
	if err != nil {
		return nil, err
	}

	return &httpPutBackend{
		pathStyleBackend: base,
		uploadURL:        uploadURL,
		tokenEnv:         tokenEnv,
		timeout:          optionalDurationSeconds(cfg, "timeoutSeconds", 600*time.Second),
	}, nil
}

func (b *httpPutBackend) Describe() string {
	if b.uploadURL == b.baseURL {
		return fmt.Sprintf("%s (http-put %s)", b.Name(), b.baseURL)
	}
	return fmt.Sprintf("%s (http-put, PUT %s, serving %s)", b.Name(), b.uploadURL, b.baseURL)
}

func (b *httpPutBackend) URLsAreLive() bool {
	return true
}

func (b *httpPutBackend) Writable() bool {
	return true
}

func (b *httpPutBackend) PutArtifact(localPath string, key string) ([]string, error) {
	token, err := b.token()
	if err != nil {
		return nil, err
	}
	_, err = httpx.PutFile(b.uploadTarget(key), localPath, token, b.timeout, contentTypeFor(key))
	if err != nil {
		return nil, err
	}
	return []string{*b.URLFor(key)}, nil
}

func (b *httpPutBackend) PutImmutable(data []byte, key string) ([]string, error) {
	token, err := b.token()
	if err != nil {
		return nil, err
	}
	_, err = httpx.PutBytes(b.uploadTarget(key), data, token, b.timeout, contentTypeFor(key))
	if err != nil {
		return nil, err
	}
	return []string{*b.URLFor(key)}, nil
}

func (b *httpPutBackend) PutPointer(data []byte, key string) ([]string, error) {
	token, err := b.token()
	if err != nil {
		return nil, err
	}
	_, err = httpx.PutBytes(b.uploadTarget(key), data, token, b.timeout, contentTypeFor(key))
	if err != nil {
		return nil, err
	}
	return []string{*b.URLFor(key)}, nil
}

func (b *httpPutBackend) Get(key string) ([]byte, error) {
	timeout := b.timeout
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	return httpx.Get(*b.URLFor(key), timeout, strings.HasPrefix(key, "index/"))
}

func (b *httpPutBackend) Probe(rawURL string) (bool, *int64, string) {
	timeout := b.timeout
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	return httpx.Probe(rawURL, timeout)
}

func (b *httpPutBackend) uploadTarget(key string) string {
	value := b.uploadURL + url.PathEscape(key)
	return strings.ReplaceAll(value, "%2F", "/")
}

func (b *httpPutBackend) token() (string, error) {
	token := os.Getenv(b.tokenEnv)
	if token == "" {
		return "", Error{Message: fmt.Sprintf("backend %q needs the upload token in the environment variable %s, which is unset or empty", b.Name(), b.tokenEnv)}
	}
	return token, nil
}

func contentTypeFor(key string) string {
	for suffix, value := range contentTypes {
		if strings.HasSuffix(key, suffix) {
			return value
		}
	}
	return "application/octet-stream"
}
