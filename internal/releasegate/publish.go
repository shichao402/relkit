package releasegate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/casput"
)

// TokenEnv is the CI-injected agent Bearer.
const TokenEnv = "RELKIT_UPLOAD_TOKEN"

// AgentSecretNote is where the token lives outside CI (local file).
const AgentSecretNote = ".relkit/secrets/agent.env"

// sanitizeUploadToken mirrors release.py sanitize_upload_token: strip BOM,
// quotes, and embedded newlines.
func sanitizeUploadToken(raw string) string {
	token := strings.ReplaceAll(raw, "\ufeff", "")
	token = strings.TrimSpace(token)
	if len(token) >= 2 && token[0] == token[len(token)-1] && (token[0] == '\'' || token[0] == '"') {
		token = strings.TrimSpace(token[1 : len(token)-1])
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(token, "\r", ""), "\n", ""))
}

// extractExport finds `export RELKIT_UPLOAD_TOKEN=...` in a notes file.
func extractExport(env, text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		prefix := "export " + env + "="
		if strings.HasPrefix(line, prefix) {
			return sanitizeUploadToken(strings.Trim(strings.TrimPrefix(line, prefix), `"'`))
		}
	}
	return ""
}

// UploadToken mirrors release.py upload_token: env first (CI secret), then
// the local secrets note.
func UploadToken(root string) (string, error) {
	token := sanitizeUploadToken(os.Getenv(TokenEnv))
	if strings.Contains(token, "${{") || strings.Contains(token, "secretKey") {
		return "", fmt.Errorf("%s looks unsubstituted by the pipeline; the CI GUI must inject the product agent Bearer", TokenEnv)
	}
	if token != "" {
		fmt.Printf("%s length=%d\n", TokenEnv, len(token))
		return token, nil
	}
	notePath := filepath.ToSlash(filepath.Join(root, AgentSecretNote))
	if data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(AgentSecretNote))); err == nil {
		if found := extractExport(TokenEnv, string(data)); found != "" {
			return found, nil
		}
	}
	return "", fmt.Errorf("%s is unset. CI injects it as a pipeline secret; locally it lives in %s", TokenEnv, notePath)
}

// redactSecrets keeps tokens out of printed output.
var bearerPattern = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/=-]{8,}`)

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func redactSecrets(text string) string {
	return bearerPattern.ReplaceAllString(text, "Bearer ***redacted***")
}

// AgentBaseURL mirrors release.py agent_base_url, with the same env-first
// precedence cas-put already uses (RELKIT_AGENT_URL beats relkit.json
// agent.url; CI passes it as a secret since products with a serve backend
// may not have an agent block at all).
func AgentBaseURL(root string) string {
	if url := strings.TrimSpace(os.Getenv("RELKIT_AGENT_URL")); url != "" {
		return url
	}
	config, err := readJSONMap(filepath.Join(root, "relkit.json"))
	if err != nil {
		return ""
	}
	agent, _ := config["agent"].(map[string]any)
	url, _ := agent["url"].(string)
	return strings.TrimSpace(url)
}

// PublishViaAgent mirrors release.py publish_via_agent: cas-put the staged
// tree, then POST the publish request; the signing key stays on the publish
// host. Unlike the Python side (which shells out to its own CLI and regexes
// the sha back out of stdout), this calls casput.Put in-process and uses
// the returned StagedSHA256 directly.
func PublishViaAgent(root, version string, execute bool) error {
	staged := filepath.Join(root, ".relkit", "cache", "staged", version)
	if info, err := os.Stat(staged); err != nil || !info.IsDir() {
		return fmt.Errorf("no staged tree for %s; run relkit stage first (%s)", version, filepath.ToSlash(staged))
	}
	state, err := LoadState(root)
	if err != nil {
		return err
	}
	product, err := state.ProductID()
	if err != nil {
		return err
	}
	url := AgentBaseURL(root)
	if url == "" {
		return fmt.Errorf("relkit.json agent.url is required for ci release --execute")
	}
	publish := strings.TrimRight(url, "/") + "/publish"
	fmt.Printf("agent %s\n", url)
	fmt.Printf("staged %s\n", filepath.ToSlash(staged))
	if !execute {
		fmt.Printf("plan: relkit cas-put --version %s --product %s\n", version, product)
		fmt.Printf("plan: POST %s\n", publish)
		fmt.Println("pass --execute to upload and publish; the signing key stays on the publish host")
		return nil
	}
	token, err := UploadToken(root)
	if err != nil {
		return err
	}
	result, err := casput.Put(context.Background(), casput.Options{
		Root:    root,
		Product: product,
		Version: version,
		URL:     url,
		Token:   token,
		Log:     func(line string) { fmt.Fprintln(os.Stderr, redactSecrets(line)) },
	})
	if err != nil {
		detail := redactSecrets(err.Error())
		if strings.Contains(detail, "401") {
			return fmt.Errorf("relkit cas-put failed\n%s\n%s is not accepted as the agent Bearer for this product", detail, TokenEnv)
		}
		return fmt.Errorf("relkit cas-put failed\n%s", detail)
	}
	fmt.Printf("cas uploaded=%d skipped=%d; staged %s/%s sha256=%s bytes=%d\n",
		result.Uploaded, result.Skipped, product, version, result.StagedSHA256, result.StagedBytes)
	if result.StagedSHA256 == "" {
		return fmt.Errorf("cas-put did not report the staged sha256")
	}

	minimum, maximum := PublishProtocolWindow(root, 2)
	idempotencyKey := fmt.Sprintf("%s/%s/%s", product, version, result.StagedSHA256)
	payload, err := jsonMarshal(map[string]string{
		"product":       product,
		"version":       version,
		"stagedSha256":  result.StagedSHA256,
		"idempotencyKey": idempotencyKey,
	})
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, publish, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Relkit-Publish-Protocol", fmt.Sprint(minimum))
	request.Header.Set("X-Relkit-Publish-Protocol-Min", fmt.Sprint(minimum))
	request.Header.Set("X-Relkit-Publish-Protocol-Max", fmt.Sprint(maximum))
	request.Header.Set("X-Relkit-Version", "relkit-cli")
	fmt.Printf("POST %s\n", publish)
	client := &http.Client{Timeout: 600 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("agent publish failed: %s: %v", publish, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		detail := redactSecrets(strings.TrimSpace(string(body)))
		return fmt.Errorf("agent publish failed: HTTP %d %s: %s", response.StatusCode, publish, detail)
	}
	if text := strings.TrimSpace(string(body)); text != "" {
		fmt.Println(redactSecrets(text))
	}
	return nil
}
