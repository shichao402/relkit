package backends

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/httpx"
)

var cosRegionFromHost = regexp.MustCompile(`(?i)^(?:.*\.)?cos\.([a-z0-9-]+)\.(?:myqcloud\.com|tencentcos\.cn)$`)

type s3CompatibleBackend struct {
	*pathStyleBackend
	endpoint       string
	bucket         string
	prefix         string
	accessKeyEnv   string
	secretKeyEnv   string
	region         string
	forcePathStyle bool
	timeout        time.Duration
}

func newS3CompatibleBackend(name string, cfg map[string]any, root string) (Backend, error) {
	_ = root
	base, err := newPathStyleBackend(name, "s3-compatible", cfg)
	if err != nil {
		return nil, err
	}

	endpoint, err := requiredString(cfg, "endpoint", name)
	if err != nil {
		return nil, err
	}
	endpoint = strings.TrimRight(endpoint, "/")
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return nil, Error{Message: fmt.Sprintf("endpoint of backend %q must be an absolute http(s) URL, got %q", name, endpoint)}
	}

	bucket, err := requiredString(cfg, "bucket", name)
	if err != nil {
		return nil, err
	}
	accessKeyEnv, err := requiredString(cfg, "accessKeyEnv", name)
	if err != nil {
		return nil, err
	}
	secretKeyEnv, err := requiredString(cfg, "secretKeyEnv", name)
	if err != nil {
		return nil, err
	}

	prefix := optionalString(cfg, "prefix")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if strings.HasPrefix(prefix, "/") {
		return nil, Error{Message: fmt.Sprintf("prefix of backend %q must not start with '/', got %q", name, prefix)}
	}

	region := optionalString(cfg, "region")
	if region == "" {
		region = regionFromEndpoint(endpoint)
	}
	if region == "" {
		return nil, Error{Message: fmt.Sprintf("backend %q needs \"region\" (could not derive it from endpoint %q)", name, endpoint)}
	}

	forcePathStyle := optionalBool(cfg, "forcePathStyle", false)
	// Local/custom endpoints usually expect path-style addressing.
	if !forcePathStyle && !isVirtualHostFriendly(endpoint) {
		forcePathStyle = true
	}

	return &s3CompatibleBackend{
		pathStyleBackend: base,
		endpoint:         endpoint,
		bucket:           bucket,
		prefix:           prefix,
		accessKeyEnv:     accessKeyEnv,
		secretKeyEnv:     secretKeyEnv,
		region:           region,
		forcePathStyle:   forcePathStyle,
		timeout:          optionalDurationSeconds(cfg, "timeoutSeconds", 600*time.Second),
	}, nil
}

func (b *s3CompatibleBackend) Describe() string {
	return fmt.Sprintf("%s (s3-compatible %s/%s -> serving %s)", b.Name(), b.endpoint, b.bucket, b.baseURL)
}

func (b *s3CompatibleBackend) URLsAreLive() bool {
	return true
}

func (b *s3CompatibleBackend) Writable() bool {
	return true
}

func (b *s3CompatibleBackend) PutArtifact(localPath string, key string) ([]string, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := b.put(key, file, info.Size(), unsignedPayload, contentTypeFor(key)); err != nil {
		return nil, err
	}
	return []string{*b.URLFor(key)}, nil
}

func (b *s3CompatibleBackend) PutImmutable(data []byte, key string) ([]string, error) {
	if err := b.put(key, bytes.NewReader(data), int64(len(data)), hashSHA256Hex(data), contentTypeFor(key)); err != nil {
		return nil, err
	}
	return []string{*b.URLFor(key)}, nil
}

func (b *s3CompatibleBackend) PutPointer(data []byte, key string) ([]string, error) {
	return b.PutImmutable(data, key)
}

func (b *s3CompatibleBackend) Get(key string) ([]byte, error) {
	accessKey, secretKey, err := b.credentials()
	if err != nil {
		return nil, err
	}
	req, err := b.newObjectRequest(http.MethodGet, key, nil, 0)
	if err != nil {
		return nil, err
	}
	if err := signAWSV4(req, emptyPayloadHash, b.region, "s3", accessKey, secretKey, time.Now()); err != nil {
		return nil, err
	}

	timeout := b.timeout
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, Error{Message: fmt.Sprintf("GET %s failed: %v", req.URL.String(), err)}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		detail := strings.TrimSpace(string(body))
		if len(detail) > 200 {
			detail = detail[:200]
		}
		msg := fmt.Sprintf("GET %s returned %d", req.URL.String(), resp.StatusCode)
		if detail != "" {
			msg += ": " + detail
		}
		return nil, Error{Message: msg}
	}
	return body, nil
}

func (b *s3CompatibleBackend) Probe(rawURL string) (bool, *int64, string) {
	timeout := b.timeout
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	return httpx.Probe(rawURL, timeout)
}

func (b *s3CompatibleBackend) put(key string, body io.Reader, size int64, payloadHash string, contentType string) error {
	accessKey, secretKey, err := b.credentials()
	if err != nil {
		return err
	}
	req, err := b.newObjectRequest(http.MethodPut, key, body, size)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	if err := signAWSV4(req, payloadHash, b.region, "s3", accessKey, secretKey, time.Now()); err != nil {
		return err
	}

	client := &http.Client{
		Timeout: b.timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return Error{Message: fmt.Sprintf("PUT %s failed: %v", req.URL.String(), err)}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return Error{Message: fmt.Sprintf("PUT %s was redirected to %s; point the backend endpoint at the final address instead", req.URL.String(), resp.Header.Get("Location"))}
	}
	if resp.StatusCode >= 400 {
		detail := strings.TrimSpace(string(respBody))
		if len(detail) > 200 {
			detail = detail[:200]
		}
		msg := fmt.Sprintf("PUT %s returned %d", req.URL.String(), resp.StatusCode)
		if detail != "" {
			msg += ": " + detail
		}
		return Error{Message: msg}
	}
	return nil
}

func (b *s3CompatibleBackend) newObjectRequest(method string, key string, body io.Reader, size int64) (*http.Request, error) {
	objectKey := b.objectKey(key)
	rawURL, err := b.objectURL(objectKey)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", httpx.UserAgent)
	if body != nil {
		req.ContentLength = size
	}
	return req, nil
}

func (b *s3CompatibleBackend) objectKey(key string) string {
	return b.prefix + key
}

func (b *s3CompatibleBackend) objectURL(objectKey string) (string, error) {
	endpointURL, err := url.Parse(b.endpoint)
	if err != nil {
		return "", err
	}
	escapedKey := escapeObjectKey(objectKey)
	if b.forcePathStyle {
		path := "/" + b.bucket
		if escapedKey != "" {
			path += "/" + escapedKey
		}
		endpointURL.Path = path
		endpointURL.RawPath = path
		endpointURL.RawQuery = ""
		return endpointURL.String(), nil
	}

	endpointURL.Host = b.bucket + "." + endpointURL.Host
	path := "/"
	if escapedKey != "" {
		path += escapedKey
	}
	endpointURL.Path = path
	endpointURL.RawPath = path
	endpointURL.RawQuery = ""
	return endpointURL.String(), nil
}

func (b *s3CompatibleBackend) credentials() (string, string, error) {
	accessKey := os.Getenv(b.accessKeyEnv)
	if accessKey == "" {
		return "", "", Error{Message: fmt.Sprintf("backend %q needs the access key in the environment variable %s, which is unset or empty", b.Name(), b.accessKeyEnv)}
	}
	secretKey := os.Getenv(b.secretKeyEnv)
	if secretKey == "" {
		return "", "", Error{Message: fmt.Sprintf("backend %q needs the secret key in the environment variable %s, which is unset or empty", b.Name(), b.secretKeyEnv)}
	}
	return accessKey, secretKey, nil
}

func escapeObjectKey(key string) string {
	if key == "" {
		return ""
	}
	parts := strings.Split(key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func regionFromEndpoint(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	if match := cosRegionFromHost.FindStringSubmatch(host); len(match) == 2 {
		return match[1]
	}
	return ""
}

func isVirtualHostFriendly(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return strings.HasSuffix(host, ".myqcloud.com") ||
		strings.HasSuffix(host, ".tencentcos.cn") ||
		strings.HasSuffix(host, ".amazonaws.com")
}

func optionalBool(cfg map[string]any, field string, fallback bool) bool {
	value, ok := cfg[field]
	if !ok || value == nil {
		return fallback
	}
	asBool, ok := value.(bool)
	if !ok {
		return fallback
	}
	return asBool
}
