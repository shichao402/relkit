package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const sslAPIVersion = "2019-12-05"

type sslClient struct {
	secretID  string
	secretKey string
	http      *http.Client
	now       func() time.Time
	sleep     func(time.Duration)
}

func newSSLClientFromEnv() *sslClient {
	id := os.Getenv("TENCENTCLOUD_SECRET_ID")
	key := os.Getenv("TENCENTCLOUD_SECRET_KEY")
	if id == "" || key == "" {
		return nil
	}
	return &sslClient{
		secretID:  id,
		secretKey: key,
		http:      &http.Client{Timeout: 30 * time.Second},
		now:       time.Now,
		sleep:     time.Sleep,
	}
}

func (c *sslClient) call(action string, payload any) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	host := "ssl.tencentcloudapi.com"
	ts := c.now().UTC()
	timestamp := strconv.FormatInt(ts.Unix(), 10)
	auth := tc3Sign("ssl", host, string(body), timestamp, ts.Format("2006-01-02"), c.secretID, c.secretKey)
	req, err := http.NewRequest(http.MethodPost, "https://"+host, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", sslAPIVersion)
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("Authorization", auth)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Response json.RawMessage `json:"Response"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("ssl %s: %s", action, strings.TrimSpace(string(raw)))
	}
	var errObj struct {
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	}
	_ = json.Unmarshal(envelope.Response, &errObj)
	if errObj.Error != nil && errObj.Error.Code != "" {
		return nil, fmt.Errorf("%s: %s", errObj.Error.Code, errObj.Error.Message)
	}
	return envelope.Response, nil
}

func tc3Sign(service, host, payload, timestamp, date, secretID, secretKey string) string {
	hashedPayload := sha256Hex(payload)
	canonicalHeaders := "content-type:application/json; charset=utf-8\nhost:" + host + "\n"
	signedHeaders := "content-type;host"
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + hashedPayload
	scope := date + "/" + service + "/tc3_request"
	stringToSign := "TC3-HMAC-SHA256\n" + timestamp + "\n" + scope + "\n" + sha256Hex(canonicalRequest)
	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	sig := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	return "TC3-HMAC-SHA256 Credential=" + secretID + "/" + scope + ", SignedHeaders=" + signedHeaders + ", Signature=" + sig
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(data))
	return m.Sum(nil)
}

func (c *sslClient) ApplyAndDeploy(t Target) error {
	resp, err := c.call("ApplyCertificate", map[string]any{
		"DvAuthMethod": "DNS_AUTO",
		"DomainName":   t.Domain,
		"PackageType":  "83",
	})
	if err != nil {
		return err
	}
	var applied struct {
		CertificateId string `json:"CertificateId"`
	}
	if err := json.Unmarshal(resp, &applied); err != nil || applied.CertificateId == "" {
		return fmt.Errorf("ApplyCertificate: missing CertificateId")
	}
	deadline := c.now().Add(3 * time.Minute)
	for c.now().Before(deadline) {
		detail, err := c.call("DescribeCertificateDetail", map[string]any{"CertificateId": applied.CertificateId})
		if err != nil {
			return err
		}
		var st struct {
			Status int `json:"Status"`
		}
		if err := json.Unmarshal(detail, &st); err != nil {
			return err
		}
		if st.Status == 1 {
			_, err = c.call("DeployCertificateInstance", map[string]any{
				"CertificateId":  applied.CertificateId,
				"ResourceType":   "cos",
				"InstanceIdList": []string{t.Region + "|" + t.Bucket + "|" + t.Domain},
			})
			return err
		}
		c.sleep(5 * time.Second)
	}
	return fmt.Errorf("timeout waiting for certificate %s", applied.CertificateId)
}

type liveAPI struct {
	client *sslClient
}

func (a liveAPI) ApplyAndDeploy(t Target) error {
	if a.client == nil {
		return fmt.Errorf("TENCENTCLOUD_SECRET_ID/KEY not set")
	}
	return a.client.ApplyAndDeploy(t)
}
