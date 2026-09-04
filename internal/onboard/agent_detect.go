package onboard

import (
	"bytes"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/envelope"
)

type agentFileConfig struct {
	UploadToken     string `json:"uploadToken"`
	UploadTokenFile string `json:"uploadTokenFile"`
	UploadTokens    []struct {
		File     string   `json:"file"`
		Products []string `json:"products"`
	} `json:"uploadTokens"`
	Products map[string]struct {
		Root    string `json:"root"`
		Profile string `json:"profile"`
	} `json:"products"`
}

func detectAgent(kind string, opts EvalOptions, res *ItemResult) bool {
	switch kind {
	case "agent-no-instance-token", "agent-product", "agent-profile", "agent-token",
		"agent-second-s3", "agent-s3-get", "agent-directory-match", "agent-cert-timer", "agent-cert-days":
	default:
		return false
	}
	configPath := opts.AgentConfig
	product := opts.Product
	if kind == "agent-cert-timer" {
		probeCertTimer(opts, res)
		return true
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		res.Status = StatusFailed
		res.Evidence = err.Error()
		return true
	}
	var cfg agentFileConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		res.Status = StatusFailed
		res.Evidence = err.Error()
		return true
	}
	dir := filepath.Dir(configPath)
	switch kind {
	case "agent-no-instance-token":
		if cfg.UploadToken != "" || cfg.UploadTokenFile != "" {
			res.Status = StatusFailed
			res.Evidence = "instance-wide token fields are still set"
			res.Remediation = "delete uploadToken / uploadTokenFile; use per-product tokens"
			return true
		}
		res.Status = StatusPassed
		res.Evidence = "no instance-wide token"
	case "agent-product":
		if product == "" {
			res.Status = StatusFailed
			res.Evidence = "-product is required"
			return true
		}
		if _, ok := cfg.Products[product]; !ok {
			res.Status = StatusFailed
			res.Evidence = fmt.Sprintf("product %q is not registered", product)
			res.Remediation = "relkit-agent init -product " + product
			return true
		}
		res.Status = StatusPassed
		res.Evidence = product
	case "agent-profile":
		path, err := profilePath(cfg, dir, product)
		if err != nil {
			res.Status = StatusFailed
			res.Evidence = err.Error()
			return true
		}
		if _, err := os.Stat(path); err != nil {
			res.Status = StatusFailed
			res.Evidence = err.Error()
			res.Remediation = "write " + path
			return true
		}
		res.Status = StatusPassed
		res.Evidence = path
	case "agent-token":
		found := ""
		for _, tok := range cfg.UploadTokens {
			for _, id := range tok.Products {
				if id == product {
					found = tok.File
				}
			}
		}
		if found == "" {
			res.Status = StatusFailed
			res.Evidence = "no token file for product"
			return true
		}
		path := found
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, found)
		}
		if _, err := os.Stat(path); err != nil {
			res.Status = StatusFailed
			res.Evidence = err.Error()
			return true
		}
		res.Status = StatusPassed
		res.Evidence = path
	case "agent-second-s3":
		profile, err := loadAgentProfile(cfg, dir, product)
		if err != nil {
			res.Status = StatusFailed
			res.Evidence = err.Error()
			return true
		}
		n := countS3Backends(profile.Backends)
		if n < 2 {
			res.Status = StatusNA
			res.Evidence = "profile has fewer than two s3-compatible backends"
			return true
		}
		if err := checkDistinctS3Map(profile.Backends); err != nil {
			res.Status = StatusFailed
			res.Evidence = err.Error()
			return true
		}
		res.Status = StatusPassed
		res.Evidence = fmt.Sprintf("%d s3-compatible backends", n)
	case "agent-s3-get":
		if opts.SkipRemote {
			res.Status = StatusNA
			res.Evidence = "remote probes skipped"
			return true
		}
		profile, err := loadAgentProfile(cfg, dir, product)
		if err != nil {
			res.Status = StatusFailed
			res.Evidence = err.Error()
			return true
		}
		urls := s3BaseURLs(profile.Backends)
		if len(urls) == 0 {
			res.Status = StatusNA
			res.Evidence = "no s3-compatible backends"
			return true
		}
		for _, u := range urls {
			if _, err := httpGet(opts, strings.TrimRight(u, "/")+"/directory/"+product+".pb"); err != nil {
				res.Status = StatusFailed
				res.Evidence = u + ": " + err.Error()
				res.Remediation = "confirm anonymous GET and custom-domain certificate"
				return true
			}
		}
		res.Status = StatusPassed
		res.Evidence = fmt.Sprintf("GET ok for %d backends", len(urls))
	case "agent-directory-match":
		if opts.SkipRemote {
			res.Status = StatusNA
			res.Evidence = "remote probes skipped"
			return true
		}
		profile, err := loadAgentProfile(cfg, dir, product)
		if err != nil {
			res.Status = StatusFailed
			res.Evidence = err.Error()
			return true
		}
		urls := profile.EntryURLs
		if len(urls) == 0 {
			urls = directoryURLsFromBackends(profile.Backends, product)
		}
		if len(urls) < 2 {
			res.Status = StatusNA
			res.Evidence = "fewer than two directory entry URLs"
			return true
		}
		var first []byte
		for i, u := range urls {
			body, err := httpGet(opts, u)
			if err != nil {
				res.Status = StatusFailed
				res.Evidence = u + ": " + err.Error()
				return true
			}
			if i == 0 {
				first = body
				continue
			}
			if !bytes.Equal(first, body) {
				res.Status = StatusFailed
				res.Evidence = "directory bytes differ across entryUrls"
				return true
			}
		}
		if err := verifyDirectoryEnvelope(first, cfg, dir, product); err != nil {
			res.Status = StatusFailed
			res.Evidence = "bytes match but signature failed: " + err.Error()
			return true
		}
		res.Status = StatusPassed
		res.Evidence = fmt.Sprintf("%d entry urls match and verify (%d bytes)", len(urls), len(first))
	case "agent-cert-days":
		if opts.SkipRemote {
			res.Status = StatusNA
			res.Evidence = "remote probes skipped"
			return true
		}
		profile, err := loadAgentProfile(cfg, dir, product)
		if err != nil {
			res.Status = StatusFailed
			res.Evidence = err.Error()
			return true
		}
		domains := s3Domains(profile.Backends)
		if len(domains) == 0 {
			res.Status = StatusNA
			res.Evidence = "no s3 custom domains"
			return true
		}
		for _, domain := range domains {
			days, err := certDaysLeft(opts, domain)
			if err != nil {
				res.Status = StatusFailed
				res.Evidence = domain + ": " + err.Error()
				return true
			}
			if days < 14 {
				res.Status = StatusFailed
				res.Evidence = fmt.Sprintf("%s: %d days remaining", domain, days)
				res.Remediation = "relkit-cos-cert-renew"
				return true
			}
		}
		res.Status = StatusPassed
		res.Evidence = fmt.Sprintf("%d domains have >=14 days", len(domains))
	}
	return true
}

func probeCertTimer(opts EvalOptions, res *ItemResult) {
	run := opts.Systemctl
	if run == nil {
		run = defaultSystemctl
	}
	out, err := run("is-enabled", "relkit-cos-cert-renew.timer")
	if err != nil {
		path := opts.CertTimerPath
		if path == "" {
			path = "/etc/systemd/system/relkit-cos-cert-renew.timer"
		}
		if _, statErr := os.Stat(path); statErr != nil {
			res.Status = StatusNA
			res.Evidence = "cert renew timer not installed (systemctl unavailable)"
			res.Remediation = "install deploy/relkit-cos-cert-renew.timer"
			return
		}
		res.Status = StatusPassed
		res.Evidence = "timer unit present; systemctl not available to confirm enablement"
		return
	}
	if strings.TrimSpace(out) != "enabled" {
		res.Status = StatusFailed
		res.Evidence = "timer is-enabled: " + strings.TrimSpace(out)
		res.Remediation = "systemctl enable --now relkit-cos-cert-renew.timer"
		return
	}
	last, _ := run("show", "relkit-cos-cert-renew.service", "-p", "ExecMainStatus", "--value")
	res.Status = StatusPassed
	res.Evidence = "enabled; last ExecMainStatus=" + strings.TrimSpace(last)
}

type agentProfile struct {
	Backends  map[string]map[string]any
	EntryURLs []string
	raw       map[string]any
}

func profilePath(cfg agentFileConfig, dir, product string) (string, error) {
	entry, ok := cfg.Products[product]
	if !ok {
		return "", fmt.Errorf("product missing")
	}
	profile := entry.Profile
	if profile == "" {
		profile = filepath.Join(dir, "products", product+".json")
	}
	if !filepath.IsAbs(profile) {
		profile = filepath.Join(dir, profile)
	}
	return profile, nil
}

func loadAgentProfile(cfg agentFileConfig, dir, product string) (*agentProfile, error) {
	path, err := profilePath(cfg, dir, product)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := &agentProfile{Backends: map[string]map[string]any{}, raw: doc}
	if backends, ok := doc["backends"].(map[string]any); ok {
		for name, v := range backends {
			if obj, ok := v.(map[string]any); ok {
				out.Backends[name] = obj
			}
		}
	}
	if dirObj, ok := doc["directory"].(map[string]any); ok {
		if urls, ok := dirObj["entryUrls"].([]any); ok {
			for _, u := range urls {
				if s, ok := u.(string); ok && s != "" {
					out.EntryURLs = append(out.EntryURLs, s)
				}
			}
		}
	}
	return out, nil
}

func s3BaseURLs(backends map[string]map[string]any) []string {
	var urls []string
	for _, b := range backends {
		if typ, _ := b["type"].(string); typ != "s3-compatible" {
			continue
		}
		u, _ := b["baseUrl"].(string)
		if u != "" {
			urls = append(urls, u)
		}
	}
	return urls
}

func directoryURLsFromBackends(backends map[string]map[string]any, product string) []string {
	var urls []string
	for _, base := range s3BaseURLs(backends) {
		urls = append(urls, strings.TrimRight(base, "/")+"/directory/"+product+".pb")
	}
	return urls
}

func checkDistinctS3Map(backends map[string]map[string]any) error {
	seen := map[string]string{}
	for name, backend := range backends {
		typ, _ := backend["type"].(string)
		if typ != "s3-compatible" {
			continue
		}
		bucket, _ := backend["bucket"].(string)
		region, _ := backend["region"].(string)
		if bucket == "" {
			return fmt.Errorf("backend %q is s3-compatible but has no bucket", name)
		}
		key := strings.ToLower(strings.TrimSpace(bucket) + "|" + strings.TrimSpace(region))
		if other, ok := seen[key]; ok {
			return fmt.Errorf("backends %q and %q share bucket %s in %s", other, name, bucket, region)
		}
		seen[key] = name
	}
	return nil
}

func httpGet(opts EvalOptions, rawURL string) ([]byte, error) {
	if opts.HTTPGet != nil {
		return opts.HTTPGet(rawURL)
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func defaultSystemctl(args ...string) (string, error) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return "", err
	}
	cmd := exec.Command("systemctl", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func certDaysLeft(opts EvalOptions, domain string) (int, error) {
	if opts.CertDays != nil {
		return opts.CertDays(domain)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(domain, "443"), &tls.Config{ServerName: domain})
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return 0, fmt.Errorf("no peer certificates")
	}
	return int(certs[0].NotAfter.Sub(time.Now()).Hours() / 24), nil
}

func s3Domains(backends map[string]map[string]any) []string {
	seen := map[string]bool{}
	var domains []string
	for _, raw := range s3BaseURLs(backends) {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			continue
		}
		host := u.Hostname()
		if seen[host] {
			continue
		}
		seen[host] = true
		domains = append(domains, host)
	}
	return domains
}

func verifyDirectoryEnvelope(body []byte, cfg agentFileConfig, dir, product string) error {
	keys, err := loadAgentTrustedKeys(cfg, dir, product)
	if err != nil {
		return err
	}
	env, err := rupv2.UnmarshalEnvelope(body)
	if err != nil {
		return fmt.Errorf("envelope: %w", err)
	}
	if _, err := envelope.OpenDirectoryEnvelope(env, keys); err != nil {
		return err
	}
	return nil
}

func loadAgentTrustedKeys(cfg agentFileConfig, dir, product string) (map[string]ed25519.PublicKey, error) {
	entry, ok := cfg.Products[product]
	if !ok {
		return nil, fmt.Errorf("product missing")
	}
	root := entry.Root
	if root == "" {
		root = dir
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(dir, root)
	}
	loaded, err := config.Load(filepath.Join(root, config.ConfigName))
	if err == nil {
		return loaded.TrustedPublicKeys()
	}
	profile, err := loadAgentProfile(cfg, dir, product)
	if err != nil {
		return nil, fmt.Errorf("no relkit.json at product root and %w", err)
	}
	return publicKeysFromBackendDoc(profile.raw)
}

func publicKeysFromBackendDoc(doc map[string]any) (map[string]ed25519.PublicKey, error) {
	signing, _ := doc["signing"].(map[string]any)
	rawKeys, _ := signing["publicKeys"].([]any)
	if len(rawKeys) == 0 {
		return nil, fmt.Errorf("no signing.publicKeys")
	}
	out := map[string]ed25519.PublicKey{}
	for _, item := range rawKeys {
		obj, _ := item.(map[string]any)
		id, _ := obj["keyId"].(string)
		b64, _ := obj["publicKeyBase64"].(string)
		if b64 == "" {
			b64, _ = obj["key"].(string)
		}
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil || id == "" || len(raw) != ed25519.PublicKeySize {
			continue
		}
		out[id] = ed25519.PublicKey(raw)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no usable public keys")
	}
	return out, nil
}
