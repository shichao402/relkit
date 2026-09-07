package backends

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	sigv4Algorithm   = "AWS4-HMAC-SHA256"
	emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	unsignedPayload  = "UNSIGNED-PAYLOAD"
)

// UnsignedPayload is the SigV4 hashed-payload literal COS query-auth and
// header-auth streaming PUTs both use.
const UnsignedPayload = unsignedPayload

func hashSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func deriveSigningKey(secret string, dateStamp string, region string, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

// signAWSV4 adds Authorization and related SigV4 headers to req.
// payloadHash is the hex SHA-256 of the body, emptyPayloadHash, or UNSIGNED-PAYLOAD.
func signAWSV4(req *http.Request, payloadHash string, region string, service string, accessKey string, secretKey string, now time.Time) error {
	if req.URL == nil {
		return fmt.Errorf("request URL is nil")
	}
	if payloadHash == "" {
		payloadHash = emptyPayloadHash
	}
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")

	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	signedHeaders, canonicalHeaders := canonicalHeaderBlock(req.Header)
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQuery(req.URL.Query()),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := strings.Join([]string{dateStamp, region, service, "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		sigv4Algorithm,
		amzDate,
		credentialScope,
		hashSHA256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(secretKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		sigv4Algorithm, accessKey, credentialScope, signedHeaders, signature,
	))
	return nil
}

// SignS3Request signs req with AWS SigV4 for service "s3".
// Set X-Amz-Security-Token on req before calling when using STS credentials.
func SignS3Request(req *http.Request, payloadHash, region, accessKey, secretKey string, now time.Time) error {
	return signAWSV4(req, payloadHash, region, "s3", accessKey, secretKey, now)
}

// ApplyCASSign attaches STS session token (if any) and header-authenticates req.
func ApplyCASSign(req *http.Request, sign *CASSign, now time.Time) error {
	if sign == nil {
		return nil
	}
	if sign.Algorithm != "" && sign.Algorithm != sigv4Algorithm {
		return fmt.Errorf("unsupported CAS sign algorithm %q", sign.Algorithm)
	}
	if sign.Region == "" || sign.AccessKey == "" || sign.SecretKey == "" {
		return fmt.Errorf("CAS sign is missing region or keys")
	}
	if sign.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", sign.SessionToken)
	}
	payloadHash := sign.PayloadHash
	if payloadHash == "" {
		payloadHash = unsignedPayload
	}
	return SignS3Request(req, payloadHash, sign.Region, sign.AccessKey, sign.SecretKey, now)
}

// PresignS3Request adds AWS SigV4 query authentication to req. The returned
// URL authorizes only this method, object path, and expiry window.
//
// If X-Amz-Content-Sha256 is unset, HashedPayload is UNSIGNED-PAYLOAD. Tencent
// COS query-auth verifies that literal; do not bind a real body sha256 when
// the destination is COS.
func PresignS3Request(req *http.Request, region, accessKey, secretKey string, now time.Time, ttl time.Duration) error {
	if req.URL == nil {
		return fmt.Errorf("request URL is nil")
	}
	if ttl < time.Second || ttl > 7*24*time.Hour {
		return fmt.Errorf("presign ttl must be between 1s and 168h")
	}
	now = now.UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	credentialScope := strings.Join([]string{dateStamp, region, "s3", "aws4_request"}, "/")
	payloadHash := req.Header.Get("X-Amz-Content-Sha256")
	if payloadHash == "" {
		payloadHash = unsignedPayload
	}
	signed := make(http.Header)
	signed.Set("Host", req.URL.Host)
	for name, values := range req.Header {
		lower := strings.ToLower(name)
		if lower != "content-type" && !strings.HasPrefix(lower, "x-amz-") {
			continue
		}
		for _, value := range values {
			signed.Add(name, value)
		}
	}
	signedHeaders, canonicalHeaders := canonicalHeaderBlock(signed)

	query := req.URL.Query()
	query.Set("X-Amz-Algorithm", sigv4Algorithm)
	query.Set("X-Amz-Credential", accessKey+"/"+credentialScope)
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", fmt.Sprintf("%d", int64(ttl/time.Second)))
	query.Set("X-Amz-SignedHeaders", signedHeaders)
	req.URL.RawQuery = canonicalQuery(query)

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQuery(req.URL.Query()),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	stringToSign := strings.Join([]string{
		sigv4Algorithm,
		amzDate,
		credentialScope,
		hashSHA256Hex([]byte(canonicalRequest)),
	}, "\n")
	signingKey := deriveSigningKey(secretKey, dateStamp, region, "s3")
	query = req.URL.Query()
	query.Set("X-Amz-Signature", hex.EncodeToString(hmacSHA256(signingKey, stringToSign)))
	req.URL.RawQuery = canonicalQuery(query)
	return nil
}

// SHA256Hex is the hex SHA-256 of data, for SigV4 payload hashing.
func SHA256Hex(data []byte) string {
	return hashSHA256Hex(data)
}

func canonicalURI(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func canonicalQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		vals := append([]string(nil), values[key]...)
		sort.Strings(vals)
		for _, value := range vals {
			parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(value))
		}
	}
	return strings.Join(parts, "&")
}

func canonicalHeaderBlock(header http.Header) (signedHeaders string, canonicalHeaders string) {
	type pair struct {
		name  string
		value string
	}
	pairs := make([]pair, 0, len(header))
	for name, values := range header {
		lower := strings.ToLower(name)
		if lower == "authorization" {
			continue
		}
		trimmed := make([]string, 0, len(values))
		for _, value := range values {
			trimmed = append(trimmed, strings.Join(strings.Fields(value), " "))
		}
		pairs = append(pairs, pair{name: lower, value: strings.Join(trimmed, ",")})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].name < pairs[j].name })

	names := make([]string, 0, len(pairs))
	var b strings.Builder
	for _, item := range pairs {
		names = append(names, item.name)
		b.WriteString(item.name)
		b.WriteByte(':')
		b.WriteString(item.value)
		b.WriteByte('\n')
	}
	return strings.Join(names, ";"), b.String()
}
