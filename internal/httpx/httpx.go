package httpx

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultTimeout = 30 * time.Second
	MaxRedirects   = 5
	UserAgent      = "relkit/1 (+https://spec.invalid/rup)"
)

var retryableStatus = map[int]struct{}{
	408: {}, 425: {}, 429: {}, 500: {}, 502: {}, 503: {}, 504: {},
}

type Error struct {
	Message string
	Status  int
	URL     string
}

func (e *Error) Error() string {
	return e.Message
}

func Get(rawURL string, timeout time.Duration, noCache bool) ([]byte, error) {
	target := rawURL
	headers := map[string]string{}
	if noCache {
		target = bustCache(rawURL)
		headers["Cache-Control"] = "no-cache"
	}

	status, _, body, err := request(target, http.MethodGet, nil, timeout, headers, 2, true)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status >= 400 {
		return nil, &Error{Message: fmt.Sprintf("GET %s returned %d", rawURL, status), Status: status, URL: rawURL}
	}
	return body, nil
}

func PutFile(rawURL string, path string, token string, timeout time.Duration, contentType string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return 0, err
	}

	headers := map[string]string{
		"Content-Length": strconv.FormatInt(info.Size(), 10),
		"Content-Type":   chooseContentType(contentType),
	}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}

	return put(rawURL, file, info.Size(), timeout, headers)
}

func PutBytes(rawURL string, data []byte, token string, timeout time.Duration, contentType string) (int, error) {
	headers := map[string]string{
		"Content-Length": strconv.Itoa(len(data)),
		"Content-Type":   chooseContentType(contentType),
	}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return put(rawURL, bytes.NewReader(data), int64(len(data)), timeout, headers)
}

func Probe(rawURL string, timeout time.Duration) (bool, *int64, string) {
	status, headers, _, err := request(rawURL, http.MethodHead, nil, timeout, nil, 1, true)
	if err != nil {
		return false, nil, err.Error()
	}
	if status < 400 {
		if length := headers.Get("Content-Length"); isDigits(length) {
			size, _ := strconv.ParseInt(length, 10, 64)
			return true, &size, "HEAD"
		}
		return true, nil, "HEAD without Content-Length"
	}

	var headStatus *int
	if status != http.StatusNotFound {
		headStatus = &status
	}

	rangeHeaders := map[string]string{"Range": "bytes=0-0"}
	status, headers, body, err := request(rawURL, http.MethodGet, nil, timeout, rangeHeaders, 1, true)
	if err != nil {
		return false, nil, err.Error()
	}
	_ = body
	if status == http.StatusNotFound {
		return false, nil, "404 Not Found"
	}
	if status >= 400 {
		note := fmt.Sprintf("HTTP %d", status)
		if headStatus != nil {
			note += fmt.Sprintf(" (HEAD returned %d)", *headStatus)
		}
		return false, nil, note
	}

	if contentRange := headers.Get("Content-Range"); contentRange != "" && strings.Contains(contentRange, "/") {
		total := contentRange[strings.LastIndex(contentRange, "/")+1:]
		if isDigits(total) {
			size, _ := strconv.ParseInt(total, 10, 64)
			return true, &size, "range request"
		}
	}
	if status == http.StatusOK {
		if length := headers.Get("Content-Length"); isDigits(length) {
			size, _ := strconv.ParseInt(length, 10, 64)
			return true, &size, "range ignored by server"
		}
	}
	return true, nil, "reachable, size unknown"
}

func SizeMatches(expected int64, actual *int64) bool {
	return actual == nil || *actual == expected
}

func request(rawURL string, method string, body io.Reader, timeout time.Duration, extraHeaders map[string]string, retries int, followRedirects bool) (int, http.Header, []byte, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !followRedirects {
				return http.ErrUseLastResponse
			}
			if len(via) >= MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", MaxRedirects)
			}
			if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme == "http" {
				return &Error{
					Message: fmt.Sprintf("refusing redirect that downgrades https to http: %s -> %s", via[len(via)-1].URL.String(), req.URL.String()),
					Status:  http.StatusFound,
					URL:     via[len(via)-1].URL.String(),
				}
			}
			return nil
		},
	}

	var requestBody []byte
	if body != nil {
		data, err := io.ReadAll(body)
		if err != nil {
			return 0, nil, nil, err
		}
		requestBody = data
	}

	var lastTransportErr error
	for attempt := 0; attempt <= retries; attempt++ {
		var reqBody io.Reader
		if requestBody != nil {
			reqBody = bytes.NewReader(requestBody)
		}
		req, err := http.NewRequest(method, rawURL, reqBody)
		if err != nil {
			return 0, nil, nil, err
		}
		req.Header.Set("User-Agent", UserAgent)
		for key, value := range extraHeaders {
			req.Header.Set(key, value)
		}
		if requestBody != nil {
			req.ContentLength = int64(len(requestBody))
		}

		resp, err := client.Do(req)
		if err != nil {
			var relkitErr *Error
			if errors.As(err, &relkitErr) {
				return 0, nil, nil, relkitErr
			}
			lastTransportErr = err
			if attempt < retries {
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
				continue
			}
			return 0, nil, nil, &Error{Message: fmt.Sprintf("%s %s failed: %v", method, rawURL, err), URL: rawURL}
		}

		responseBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastTransportErr = readErr
			if attempt < retries {
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
				continue
			}
			return 0, nil, nil, &Error{Message: fmt.Sprintf("%s %s failed: %v", method, rawURL, readErr), URL: rawURL}
		}

		if _, ok := retryableStatus[resp.StatusCode]; ok && attempt < retries {
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			continue
		}
		return resp.StatusCode, resp.Header, responseBody, nil
	}

	return 0, nil, nil, &Error{Message: fmt.Sprintf("%s %s failed: %v", method, rawURL, lastTransportErr), URL: rawURL}
}

func put(rawURL string, body io.Reader, size int64, timeout time.Duration, headers map[string]string) (int, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest(http.MethodPut, rawURL, body)
	if err != nil {
		return 0, err
	}
	req.ContentLength = size
	req.Header.Set("User-Agent", UserAgent)
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, &Error{Message: fmt.Sprintf("PUT %s failed: %v", rawURL, err), URL: rawURL}
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return 0, &Error{Message: fmt.Sprintf("PUT %s was redirected to %s; point the backend at the final address instead", rawURL, resp.Header.Get("Location")), Status: resp.StatusCode, URL: rawURL}
	}
	if resp.StatusCode >= 400 {
		detail := firstBodyLine(bodyBytes)
		message := fmt.Sprintf("PUT %s returned %d", rawURL, resp.StatusCode)
		if detail != "" {
			message += ": " + detail
		}
		return 0, &Error{Message: message, Status: resp.StatusCode, URL: rawURL}
	}
	return resp.StatusCode, nil
}

func bustCache(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	query.Set("t", strconv.FormatInt(time.Now().Unix(), 10))
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func firstBodyLine(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	line, _, _ := strings.Cut(text, "\n")
	if len(line) > 200 {
		line = line[:200]
	}
	return line
}

func chooseContentType(value string) string {
	if value != "" {
		return value
	}
	return "application/octet-stream"
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
