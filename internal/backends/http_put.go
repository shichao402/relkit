package backends

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"cnb.cool/shichao402/relkit/internal/httpx"
	"cnb.cool/shichao402/relkit/internal/publishproto"
)

var contentTypes = map[string]string{
	".pb":   "application/protobuf",
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

func (b *httpPutBackend) HostsBrowse() bool {
	return true
}

func (b *httpPutBackend) PutArtifact(localPath string, key string) ([]string, error) {
	token, err := b.token()
	if err != nil {
		return nil, err
	}
	_, err = httpx.PutFile(b.uploadTarget(key), localPath, token, b.timeout, contentTypeFor(key), publisherHeaders())
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
	_, err = httpx.PutBytes(b.uploadTarget(key), data, token, b.timeout, contentTypeFor(key), publisherHeaders())
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
	_, err = httpx.PutBytes(b.uploadTarget(key), data, token, b.timeout, contentTypeFor(key), publisherHeaders())
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
	return httpx.Get(*b.URLFor(key), timeout, strings.HasPrefix(key, "index/") || strings.HasPrefix(key, "fallback/") || strings.HasPrefix(key, "directory/"))
}

func (b *httpPutBackend) Probe(rawURL string) (bool, *int64, string) {
	timeout := b.timeout
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	return httpx.Probe(rawURL, timeout)
}

// Preflight asks relkit-serve to validate this publisher before any artifact is
// uploaded. A 404/405 means the target is a legacy or generic PUT endpoint; PUT
// headers still let a current relkit-serve enforce its policy authoritatively.
func (b *httpPutBackend) Preflight() error {
	token, err := b.token()
	if err != nil {
		return err
	}
	target := strings.TrimSuffix(b.uploadURL, "/") + publishproto.PreflightPath
	status, _, body, err := httpx.Post(target, token, minDuration(b.timeout, 60*time.Second), publisherHeaders())
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
		return nil
	}

	var response struct {
		OK          bool   `json:"ok"`
		MinProtocol int    `json:"minProtocol"`
		Error       string `json:"error"`
		Message     string `json:"message"`
	}
	_ = json.Unmarshal(body, &response)
	if status >= 200 && status < 300 {
		if response.MinProtocol > publishproto.Current {
			return Error{Message: fmt.Sprintf(
				"backend %q requires publish protocol %d, but this relkit supports %d; upgrade relkit",
				b.Name(), response.MinProtocol, publishproto.Current)}
		}
		return nil
	}
	if status == http.StatusUpgradeRequired || response.Error == "publisher_upgrade_required" {
		return Error{Message: fmt.Sprintf(
			"backend %q rejected this publisher: publish protocol %d is required, this relkit supports %d; upgrade relkit",
			b.Name(), response.MinProtocol, publishproto.Current)}
	}
	detail := strings.TrimSpace(response.Message)
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	if newline := strings.IndexByte(detail, '\n'); newline >= 0 {
		detail = detail[:newline]
	}
	message := fmt.Sprintf("publish preflight for backend %q returned HTTP %d", b.Name(), status)
	if detail != "" {
		message += ": " + detail
	}
	return Error{Message: message}
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

func publisherHeaders() map[string]string {
	return map[string]string{
		publishproto.ProtocolHeader: strconv.Itoa(publishproto.Current),
		publishproto.VersionHeader:  publishproto.PublisherVersion,
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= 0 || a > b {
		return b
	}
	return a
}

func contentTypeFor(key string) string {
	for suffix, value := range contentTypes {
		if strings.HasSuffix(key, suffix) {
			return value
		}
	}
	return "application/octet-stream"
}
