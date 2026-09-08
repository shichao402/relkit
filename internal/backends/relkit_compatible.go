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

const (
	relkitCopySourceHeader = "X-Relkit-Copy-Source"
	relkitCASUploadsPath   = "/-/cas/uploads"
)

var contentTypes = map[string]string{
	".pb":   "application/protobuf",
	".json": "application/json",
	".zip":  "application/zip",
	".gz":   "application/gzip",
	".tgz":  "application/gzip",
}

type relkitCompatibleBackend struct {
	*pathStyleBackend
	uploadURL string
	tokenEnv  string
	timeout   time.Duration
}

func newRelkitCompatibleBackend(name string, cfg map[string]any, root string) (Backend, error) {
	_ = root
	base, err := newPathStyleBackend(name, "relkit-compatible", cfg)
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

	return &relkitCompatibleBackend{
		pathStyleBackend: base,
		uploadURL:        uploadURL,
		tokenEnv:         tokenEnv,
		timeout:          optionalDurationSeconds(cfg, "timeoutSeconds", 600*time.Second),
	}, nil
}

func (b *relkitCompatibleBackend) Describe() string {
	if b.uploadURL == b.baseURL {
		return fmt.Sprintf("%s (relkit-compatible %s)", b.Name(), b.baseURL)
	}
	return fmt.Sprintf("%s (relkit-compatible, PUT %s, serving %s)", b.Name(), b.uploadURL, b.baseURL)
}

func (b *relkitCompatibleBackend) URLsAreLive() bool {
	return true
}

func (b *relkitCompatibleBackend) Writable() bool {
	return true
}

func (b *relkitCompatibleBackend) HostsBrowse() bool {
	return true
}

func (b *relkitCompatibleBackend) PutArtifact(localPath string, key string) ([]string, error) {
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

func (b *relkitCompatibleBackend) PutImmutable(data []byte, key string) ([]string, error) {
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

func (b *relkitCompatibleBackend) PutPointer(data []byte, key string) ([]string, error) {
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

func (b *relkitCompatibleBackend) Get(key string) ([]byte, error) {
	timeout := b.timeout
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	return httpx.Get(*b.URLFor(key), timeout, strings.HasPrefix(key, "index/") || strings.HasPrefix(key, "fallback/") || strings.HasPrefix(key, "directory/"))
}

func (b *relkitCompatibleBackend) Probe(rawURL string) (bool, *int64, string) {
	timeout := b.timeout
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	return httpx.Probe(rawURL, timeout)
}

func (b *relkitCompatibleBackend) Preflight() error {
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

func (b *relkitCompatibleBackend) AuthorizeCASUpload(req CASUploadRequest) (*CASUpload, error) {
	token, err := b.token()
	if err != nil {
		return nil, err
	}
	ttl := req.TTL
	if ttl < time.Second {
		ttl = time.Hour
	}
	body, err := json.Marshal(map[string]any{
		"key":  req.Key,
		"size": req.Size,
		"ttl":  int(ttl.Seconds()),
	})
	if err != nil {
		return nil, err
	}
	target := strings.TrimSuffix(b.uploadURL, "/") + relkitCASUploadsPath
	status, data, err := httpx.PostJSON(target, token, minDuration(b.timeout, 60*time.Second), body, publisherHeaders())
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, Error{Message: fmt.Sprintf("mint CAS upload returned HTTP %d: %s", status, strings.TrimSpace(string(data)))}
	}
	var minted struct {
		URL       string    `json:"url"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	if err := json.Unmarshal(data, &minted); err != nil || minted.URL == "" {
		return nil, Error{Message: "mint CAS upload returned invalid json"}
	}
	return SinglePUT(minted.URL, nil, minted.ExpiresAt), nil
}

func (b *relkitCompatibleBackend) CASUploadOrigin() string {
	return strings.TrimSuffix(b.uploadURL, "/")
}

func (b *relkitCompatibleBackend) Head(key string) (int64, bool, error) {
	token, err := b.token()
	if err != nil {
		return 0, false, err
	}
	return httpx.Head(b.uploadTarget(key), token, minDuration(b.timeout, 60*time.Second))
}

func (b *relkitCompatibleBackend) Promote(srcKey, dstKey string) ([]string, error) {
	token, err := b.token()
	if err != nil {
		return nil, err
	}
	headers := publisherHeaders()
	headers[relkitCopySourceHeader] = srcKey
	_, err = httpx.PutEmpty(b.uploadTarget(dstKey), token, b.timeout, headers)
	if err != nil {
		return nil, err
	}
	return []string{*b.URLFor(dstKey)}, nil
}

func (b *relkitCompatibleBackend) Delete(key string) error {
	token, err := b.token()
	if err != nil {
		return err
	}
	return httpx.Delete(b.uploadTarget(key), token, minDuration(b.timeout, 60*time.Second))
}

func (b *relkitCompatibleBackend) uploadTarget(key string) string {
	value := b.uploadURL + url.PathEscape(key)
	return strings.ReplaceAll(value, "%2F", "/")
}

func (b *relkitCompatibleBackend) token() (string, error) {
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

var _ Ingest = (*relkitCompatibleBackend)(nil)
var _ Deleter = (*relkitCompatibleBackend)(nil)
var _ CASUploadAuthorizer = (*relkitCompatibleBackend)(nil)
