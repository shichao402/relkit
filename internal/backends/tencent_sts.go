package backends

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	tencentSTSHost    = "sts.tencentcloudapi.com"
	tencentSTSVersion = "2018-08-13"
	tencentSTSService = "sts"
)

var tencentSTSEndpoint = "https://" + tencentSTSHost

type federationCredentials struct {
	SecretID   string
	SecretKey  string
	Token      string
	Expiration time.Time
}

type tencentSTSRequest struct {
	Name            string `json:"Name"`
	Policy          string `json:"Policy"`
	DurationSeconds int    `json:"DurationSeconds"`
}

type tencentSTSEnvelope struct {
	Response struct {
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
		Credentials *struct {
			Token        string `json:"Token"`
			TmpSecretId  string `json:"TmpSecretId"`
			TmpSecretKey string `json:"TmpSecretKey"`
		} `json:"Credentials"`
		Expiration  string `json:"Expiration"`
		ExpiredTime int64  `json:"ExpiredTime"`
	} `json:"Response"`
}

func getFederationToken(client *http.Client, secretID, secretKey, name, policy string, duration time.Duration) (*federationCredentials, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	seconds := int(duration / time.Second)
	if seconds < 1800 {
		seconds = 1800
	}
	if seconds > 7200 {
		seconds = 7200
	}
	payload, err := json.Marshal(tencentSTSRequest{
		Name:            name,
		Policy:          policy,
		DurationSeconds: seconds,
	})
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	req, err := http.NewRequest(http.MethodPost, tencentSTSEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", tencentSTSHost)
	req.Header.Set("X-TC-Action", "GetFederationToken")
	req.Header.Set("X-TC-Version", tencentSTSVersion)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(now.Unix(), 10))
	req.Header.Set("Authorization", tc3Authorization(secretID, secretKey, payload, now))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetFederationToken: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("GetFederationToken: read body: %w", err)
	}
	var envelope tencentSTSEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("GetFederationToken: HTTP %d: %s", resp.StatusCode, truncate(strings.TrimSpace(string(body)), 300))
	}
	if envelope.Response.Error != nil {
		return nil, fmt.Errorf("GetFederationToken %s: %s", envelope.Response.Error.Code, envelope.Response.Error.Message)
	}
	creds := envelope.Response.Credentials
	if creds == nil || creds.TmpSecretId == "" || creds.TmpSecretKey == "" || creds.Token == "" {
		return nil, fmt.Errorf("GetFederationToken: missing credentials in response")
	}
	expiry := time.Unix(envelope.Response.ExpiredTime, 0).UTC()
	if envelope.Response.Expiration != "" {
		if parsed, err := time.Parse(time.RFC3339, envelope.Response.Expiration); err == nil {
			expiry = parsed.UTC()
		}
	}
	if expiry.IsZero() {
		expiry = now.Add(time.Duration(seconds) * time.Second)
	}
	return &federationCredentials{
		SecretID:   creds.TmpSecretId,
		SecretKey:  creds.TmpSecretKey,
		Token:      creds.Token,
		Expiration: expiry,
	}, nil
}

func tc3Authorization(secretID, secretKey string, payload []byte, now time.Time) string {
	timestamp := strconv.FormatInt(now.Unix(), 10)
	date := now.Format("2006-01-02")
	hashedPayload := hashSHA256Hex(payload)
	canonicalHeaders := "content-type:application/json; charset=utf-8\nhost:" + tencentSTSHost + "\n"
	signedHeaders := "content-type;host"
	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		"/",
		"",
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	}, "\n")
	credentialScope := date + "/" + tencentSTSService + "/tc3_request"
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		timestamp,
		credentialScope,
		hashSHA256Hex([]byte(canonicalRequest)),
	}, "\n")
	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, tencentSTSService)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	return "TC3-HMAC-SHA256 Credential=" + secretID + "/" + credentialScope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func cosPutObjectPolicy(region, appID, bucket, objectKey string) string {
	resource := fmt.Sprintf("qcs::cos:%s:uid/%s:%s/%s", region, appID, bucket, objectKey)
	body, _ := json.Marshal(map[string]any{
		"version": "2.0",
		"statement": []map[string]any{{
			"effect":   "allow",
			"action":   []string{"name/cos:PutObject"},
			"resource": []string{resource},
		}},
	})
	return string(body)
}

func appIDFromBucket(bucket string) string {
	i := strings.LastIndex(bucket, "-")
	if i < 0 || i+1 >= len(bucket) {
		return ""
	}
	suffix := bucket[i+1:]
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return suffix
}
