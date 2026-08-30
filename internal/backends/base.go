package backends

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/httpx"
)

type Error struct {
	Message string
}

func (e Error) Error() string {
	return e.Message
}

type Backend interface {
	Name() string
	Type() string
	Describe() string
	URLsAreLive() bool
	Writable() bool
	PutArtifact(localPath string, key string) ([]string, error)
	PutImmutable(data []byte, key string) ([]string, error)
	PutPointer(data []byte, key string) ([]string, error)
	Get(key string) ([]byte, error)
	URLFor(key string) *string
	Probe(rawURL string) (bool, *int64, string)
	// HostsBrowse is true when this backend's GET tree can serve the human
	// index (browse/). Protocol-only stores return false; HTML goes to a
	// BrowseSink instead of this backend.
	HostsBrowse() bool
}

// PublishPreflighter is optional. Backends that can negotiate writer
// capabilities implement it; storage backends such as local and S3 do not.
type PublishPreflighter interface {
	Preflight() error
}

type baseBackend struct {
	name        string
	backendType string
}

func (b *baseBackend) Name() string {
	return b.name
}

func (b *baseBackend) Type() string {
	return b.backendType
}

type pathStyleBackend struct {
	baseBackend
	baseURL string
}

func newPathStyleBackend(name string, backendType string, cfg map[string]any) (*pathStyleBackend, error) {
	baseURL, err := requiredString(cfg, "baseUrl", name)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return nil, Error{Message: fmt.Sprintf("baseUrl of backend %q must be an absolute http(s) URL, got %q", name, baseURL)}
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &pathStyleBackend{
		baseBackend: baseBackend{name: name, backendType: backendType},
		baseURL:     baseURL,
	}, nil
}

func (b *pathStyleBackend) URLFor(key string) *string {
	value := b.baseURL + url.PathEscape(key)
	value = strings.ReplaceAll(value, "%2F", "/")
	return &value
}

func (b *pathStyleBackend) Probe(rawURL string) (bool, *int64, string) {
	return httpx.Probe(rawURL, httpx.DefaultTimeout)
}

func (b *pathStyleBackend) HostsBrowse() bool {
	return false
}

func (b *pathStyleBackend) resolveUnder(directory string, key string) (string, error) {
	base, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	target := filepath.Join(append([]string{base}, strings.Split(key, "/")...)...)
	target, err = filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", Error{Message: fmt.Sprintf("key %q escapes %s", key, base)}
	}
	return target, nil
}

func (b *pathStyleBackend) writeFile(directory string, key string, data []byte) error {
	target, err := b.resolveUnder(directory, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func (b *pathStyleBackend) copyFile(directory string, key string, source string) error {
	target, err := b.resolveUnder(directory, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(target)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func Create(name string, cfg *config.Config, root string) (Backend, error) {
	entry, err := cfg.BackendConfig(name)
	if err != nil {
		return nil, err
	}
	backendType, err := requiredString(entry, "type", name)
	if err != nil {
		return nil, err
	}
	switch backendType {
	case "local":
		return newLocalBackend(name, entry, root)
	case "static-http":
		return newStaticHTTPBackend(name, entry, root)
	case "http-put":
		return newHTTPPutBackend(name, entry, root)
	case "s3-compatible":
		return newS3CompatibleBackend(name, entry, root)
	default:
		return nil, Error{Message: fmt.Sprintf("unsupported backend type %q for backend %q (available: http-put, local, s3-compatible, static-http)", backendType, name)}
	}
}

func AvailableTypes() []string {
	return []string{"http-put", "local", "s3-compatible", "static-http"}
}

func SummaryFor(backendType string) (summary string, required []string, optional []string) {
	switch backendType {
	case "local":
		return "writes the key tree to a local directory; URLs are not expected to resolve yet", []string{"baseUrl", "outputDir"}, nil
	case "static-http":
		return "any host serving files over HTTP at a predictable path; without stageDir it is read-only", []string{"baseUrl"}, []string{"stageDir", "timeoutSeconds"}
	case "http-put":
		return "uploads with authenticated PUT and serves over HTTP; works with relkit-serve or any PUT/WebDAV endpoint", []string{"baseUrl", "tokenEnv"}, []string{"uploadUrl", "timeoutSeconds"}
	case "s3-compatible":
		return "uploads with SigV4 to COS / S3 / MinIO; clients download via baseUrl (custom domain or CDN)", []string{"baseUrl", "endpoint", "bucket", "accessKeyEnv", "secretKeyEnv"}, []string{"prefix", "region", "forcePathStyle", "timeoutSeconds"}
	default:
		return "", nil, nil
	}
}

func requiredString(cfg map[string]any, field string, backendName string) (string, error) {
	value, ok := cfg[field]
	if !ok || value == nil {
		return "", Error{Message: fmt.Sprintf("backend %q requires %q", backendName, field)}
	}
	asString, ok := value.(string)
	if !ok || asString == "" {
		return "", Error{Message: fmt.Sprintf("backend %q requires %q", backendName, field)}
	}
	return asString, nil
}

func optionalString(cfg map[string]any, field string) string {
	value, ok := cfg[field]
	if !ok || value == nil {
		return ""
	}
	asString, _ := value.(string)
	return asString
}

func optionalDurationSeconds(cfg map[string]any, field string, fallback time.Duration) time.Duration {
	value, ok := cfg[field]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return time.Duration(typed) * time.Second
	case int:
		return time.Duration(typed) * time.Second
	case int64:
		return time.Duration(typed) * time.Second
	default:
		return fallback
	}
}
