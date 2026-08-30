// Package makers deploys the human-facing browse dump to EdgeOne Pages/Makers.
// Protocol clients never read those files. Public COS stays protocol-only.
package makers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"cnb.cool/shichao402/relkit/internal/backends"
	"cnb.cool/shichao402/relkit/internal/browse"
	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/httpx"
)

const (
	maxUploadFiles = 2000
	maxUploadBytes = 64 * 1024 * 1024
	chinaAPI       = "https://pages-api.cloud.tencent.com/v1"
	globalAPI      = "https://pages-api.edgeone.ai/v1"
)

// APIBaseURL is the Pages OpenAPI endpoint for a site.makers.region value.
func APIBaseURL(region string) string {
	if strings.EqualFold(region, "global") {
		return globalAPI
	}
	return chinaAPI
}

// Object is one file uploaded to the Pages temporary COS prefix.
type Object struct {
	Bucket      string
	Region      string
	Key         string
	ContentType string
	Body        []byte
	SecretID    string
	SecretKey   string
	Token       string
}

// Client talks to the Pages API and uploads dump files with STS.
type Client struct {
	Token      string
	BaseURL    string
	HTTP       *http.Client
	Now        func() time.Time
	PutObject  func(Object) error
	COSBaseURL string
}

type tempToken struct {
	Bucket      string `json:"Bucket"`
	Region      string `json:"Region"`
	TargetPath  string `json:"TargetPath"`
	Credentials *struct {
		TmpSecretID  string `json:"TmpSecretId"`
		TmpSecretKey string `json:"TmpSecretKey"`
		Token        string `json:"Token"`
	} `json:"Credentials"`
}

type dumpFile struct {
	Rel  string
	Body []byte
}

// DeployDump uploads .relkit/browse (html + catalog.json) to the configured
// Makers project. It does not print or return the Pages token.
func DeployDump(root string, makers *config.MakersConfig) error {
	if makers == nil || makers.ProjectID == "" {
		return fmt.Errorf("site.makers.projectId is required")
	}
	tokenEnv := makers.TokenEnv
	if tokenEnv == "" {
		tokenEnv = config.DefaultMakersTokenEnv
	}
	token := os.Getenv(tokenEnv)
	if token == "" {
		return fmt.Errorf("site.makers needs the Pages token in the environment variable %s, which is unset or empty", tokenEnv)
	}
	client := &Client{
		Token:   token,
		BaseURL: APIBaseURL(makers.Region),
	}
	_, err := client.DeployDir(makers.ProjectID, browse.DumpDir(root))
	return err
}

// Result is the Pages deployment created after the dump upload.
type Result struct {
	ProjectID      string
	DeploymentID   string
	TempBucketPath string
	Uploaded       []string
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// DeployDir uploads allowed dump files and creates a Folder deployment.
func (c *Client) DeployDir(projectID, dir string) (*Result, error) {
	if c.Token == "" {
		return nil, fmt.Errorf("Pages token is empty")
	}
	if projectID == "" {
		return nil, fmt.Errorf("site.makers.projectId is required")
	}
	files, err := listDumpFiles(dir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s has no index/product HTML or catalog.json", dir)
	}

	var temp tempToken
	if err := c.call("DescribePagesCosTempToken", map[string]any{
		"ProjectId": projectID,
	}, &temp); err != nil {
		return nil, err
	}
	if temp.Credentials == nil || temp.Credentials.TmpSecretID == "" {
		return nil, fmt.Errorf("DescribePagesCosTempToken did not return temporary credentials")
	}

	prefix := strings.Trim(temp.TargetPath, "/")
	uploaded := make([]string, 0, len(files))
	for _, file := range files {
		key := file.Rel
		if prefix != "" {
			key = prefix + "/" + file.Rel
		}
		obj := Object{
			Bucket:      temp.Bucket,
			Region:      temp.Region,
			Key:         key,
			ContentType: contentTypeFor(file.Rel),
			Body:        file.Body,
			SecretID:    temp.Credentials.TmpSecretID,
			SecretKey:   temp.Credentials.TmpSecretKey,
			Token:       temp.Credentials.Token,
		}
		if err := c.putObject(obj); err != nil {
			return nil, err
		}
		uploaded = append(uploaded, file.Rel)
	}

	tempBucketPath := temp.TargetPath
	if tempBucketPath == "" {
		tempBucketPath = prefix
	}
	var deployment struct {
		DeploymentID string `json:"DeploymentId"`
	}
	if err := c.call("CreatePagesDeployment", map[string]any{
		"ProjectId":      projectID,
		"ViaMeta":        "Upload",
		"Provider":       "Upload",
		"Env":            "Production",
		"DistType":       "Folder",
		"TempBucketPath": tempBucketPath,
	}, &deployment); err != nil {
		return nil, err
	}
	return &Result{
		ProjectID:      projectID,
		DeploymentID:   deployment.DeploymentID,
		TempBucketPath: tempBucketPath,
		Uploaded:       uploaded,
	}, nil
}

func (c *Client) putObject(obj Object) error {
	if c.PutObject != nil {
		return c.PutObject(obj)
	}
	return c.putCOS(obj)
}

func (c *Client) putCOS(obj Object) error {
	if obj.Bucket == "" || obj.Region == "" {
		return fmt.Errorf("Pages temporary COS bucket/region missing")
	}
	rawURL, err := c.objectURL(obj.Bucket, obj.Region, obj.Key)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, rawURL, bytes.NewReader(obj.Body))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(obj.Body))
	req.Header.Set("User-Agent", httpx.UserAgent)
	req.Header.Set("Content-Type", obj.ContentType)
	if obj.Token != "" {
		req.Header.Set("X-Amz-Security-Token", obj.Token)
	}
	payloadHash := backends.SHA256Hex(obj.Body)
	if err := backends.SignS3Request(req, payloadHash, obj.Region, obj.SecretID, obj.SecretKey, c.now()); err != nil {
		return err
	}
	client := c.httpClient()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("PUT %s failed: %v", redactURL(req.URL), err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return fmt.Errorf("PUT %s was redirected; point at the final COS address instead", redactURL(req.URL))
	}
	if resp.StatusCode >= 400 {
		detail := strings.TrimSpace(string(body))
		msg := fmt.Sprintf("PUT %s returned %d", redactURL(req.URL), resp.StatusCode)
		if detail != "" {
			msg += ": " + detail
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func (c *Client) objectURL(bucket, region, key string) (string, error) {
	escaped := escapeObjectKey(key)
	if c.COSBaseURL != "" {
		base := strings.TrimRight(c.COSBaseURL, "/")
		return base + "/" + bucket + "/" + escaped, nil
	}
	if bucket == "" || region == "" {
		return "", fmt.Errorf("COS bucket and region are required")
	}
	return "https://" + bucket + ".cos." + region + ".myqcloud.com/" + escaped, nil
}

func (c *Client) call(action string, payload map[string]any, dest any) error {
	body := map[string]any{"Action": action}
	for key, value := range payload {
		body[key] = value
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	endpoint := c.BaseURL
	if endpoint == "" {
		endpoint = chinaAPI
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", httpx.UserAgent)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("Pages %s failed: %v", action, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("Pages %s: reading body: %v", action, err)
	}
	unwrapped, err := unwrapPagesBody(respBody)
	if err != nil {
		if resp.StatusCode >= 400 {
			return fmt.Errorf("Pages %s failed (HTTP %d): %s", action, resp.StatusCode, truncate(string(respBody), 200))
		}
		return fmt.Errorf("Pages %s: %v", action, err)
	}
	if apiErr := pagesError(unwrapped); apiErr != "" {
		return fmt.Errorf("Pages %s failed: %s", action, apiErr)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Pages %s failed (HTTP %d): %s", action, resp.StatusCode, truncate(string(respBody), 200))
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(unwrapped, dest); err != nil {
		return fmt.Errorf("Pages %s: decode: %v", action, err)
	}
	return nil
}

func unwrapPagesBody(raw []byte) ([]byte, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty response")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("non-JSON response: %s", truncate(string(raw), 200))
	}
	if codeRaw, ok := top["Code"]; ok {
		var code any
		if err := json.Unmarshal(codeRaw, &code); err == nil && !pagesOKCode(code) {
			msg := jsonString(top["Message"])
			return nil, fmt.Errorf("Code=%v %s", code, msg)
		}
	}
	if data, ok := top["Data"]; ok && len(data) > 0 && string(data) != "null" {
		var nested map[string]json.RawMessage
		if json.Unmarshal(data, &nested) == nil {
			if resp, ok := nested["Response"]; ok && len(resp) > 0 && string(resp) != "null" {
				return resp, nil
			}
		}
		return data, nil
	}
	if resp, ok := top["Response"]; ok && len(resp) > 0 && string(resp) != "null" {
		return resp, nil
	}
	return raw, nil
}

func pagesOKCode(code any) bool {
	switch v := code.(type) {
	case float64:
		return v == 0
	case json.Number:
		n, err := v.Int64()
		return err == nil && n == 0
	case string:
		s := strings.TrimSpace(v)
		return s == "" || s == "0" || strings.EqualFold(s, "ok") || strings.EqualFold(s, "success")
	default:
		return true
	}
}

func pagesError(raw []byte) string {
	var body struct {
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	}
	if json.Unmarshal(raw, &body) != nil || body.Error == nil {
		return ""
	}
	return strings.TrimSpace(body.Error.Code + " " + body.Error.Message)
}

func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

func listDumpFiles(dir string) ([]dumpFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no dump at %s", dir)
		}
		return nil, err
	}
	var files []dumpFile
	var total int
	for _, entry := range entries {
		if entry.IsDir() || !keepDumpName(entry.Name()) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		total += len(body)
		if len(files)+1 > maxUploadFiles {
			return nil, fmt.Errorf("dump has more than %d files", maxUploadFiles)
		}
		if total > maxUploadBytes {
			return nil, fmt.Errorf("dump exceeds %d bytes", maxUploadBytes)
		}
		files = append(files, dumpFile{Rel: path.Clean(entry.Name()), Body: body})
	}
	return files, nil
}

func keepDumpName(name string) bool {
	base := filepath.Base(name)
	if base == "catalog.json" {
		return true
	}
	return strings.HasSuffix(strings.ToLower(base), ".html")
}

func contentTypeFor(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	default:
		return "application/octet-stream"
	}
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

func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	copied := *u
	copied.User = nil
	copied.RawQuery = ""
	copied.Fragment = ""
	return copied.String()
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
