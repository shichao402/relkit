package casput

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.firoyang.com/relkit/internal/config"
	"go.firoyang.com/relkit/internal/httpx"
	"go.firoyang.com/relkit/internal/publishproto"
	"go.firoyang.com/relkit/internal/stage"
	"go.firoyang.com/relkit/internal/stagedput"
)

type Options struct {
	Root        string
	Product     string
	Version     string
	URL         string
	Token       string
	PartSize    int64
	Concurrency int
	HTTPClient  *http.Client
	Log         func(string)
}

type Result struct {
	Uploaded     int
	Skipped      int
	StagedSHA256 string
	StagedBytes  int64
}

type blob struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type credentialRequest struct {
	Product  string `json:"product"`
	PartSize int64  `json:"partSize,omitempty"`
	Blobs    []blob `json:"blobs"`
}

type uploadRequest struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt time.Time         `json:"expiresAt"`
	Offset    int64             `json:"offset,omitempty"`
	Length    int64             `json:"length,omitempty"`
}

type upload struct {
	SHA256   string          `json:"sha256"`
	Size     int64           `json:"size"`
	UploadID string          `json:"uploadId,omitempty"`
	Requests []uploadRequest `json:"requests"`
}

type partReport struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}

type credentialResponse struct {
	ExpiresAt time.Time `json:"expiresAt"`
	Uploads   []upload  `json:"uploads"`
}

const maxUploadAttempts = 4

func Put(ctx context.Context, opts Options) (*Result, error) {
	if opts.Root == "" || opts.Product == "" || opts.Version == "" {
		return nil, fmt.Errorf("root, product, and version are required")
	}
	staged, err := stage.LoadStaged(opts.Root, opts.Version)
	if err != nil {
		return nil, err
	}
	if staged.Product != opts.Product {
		return nil, fmt.Errorf("staged product %q does not match %q", staged.Product, opts.Product)
	}
	if mismatches := stage.VerifyStagedHashes(&config.Config{Root: opts.Root}, staged); len(mismatches) > 0 {
		return nil, fmt.Errorf("staging tree no longer matches staged.pb:\n  %s", strings.Join(mismatches, "\n  "))
	}
	if opts.URL == "" || opts.Token == "" {
		return nil, fmt.Errorf("url and token are required")
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Hour}
	}
	if err := publishproto.PreflightAgent(ctx, client, opts.URL, opts.Token, opts.Product); err != nil {
		return nil, err
	}
	request := credentialRequest{Product: opts.Product, PartSize: opts.PartSize}
	paths := make(map[string]string, len(staged.Artifacts))
	sizes := make(map[string]int64, len(staged.Artifacts))
	for _, artifact := range staged.Artifacts {
		digest := strings.ToLower(artifact.Sha256)
		if previous, ok := sizes[digest]; ok {
			if previous != artifact.Size {
				return nil, fmt.Errorf("staged sha256 %s has conflicting sizes", digest)
			}
			continue
		}
		request.Blobs = append(request.Blobs, blob{SHA256: artifact.Sha256, Size: artifact.Size})
		sizes[digest] = artifact.Size
		paths[digest] = filepath.Join(stage.ArtifactsDir(opts.Root, opts.Version), artifact.Filename)
	}
	credentials, err := fetchCredentials(ctx, client, opts, request)
	if err != nil {
		return nil, err
	}
	if !credentials.ExpiresAt.IsZero() && time.Now().After(credentials.ExpiresAt) {
		return nil, fmt.Errorf("CAS credentials already expired at %s", credentials.ExpiresAt.Format(time.RFC3339))
	}
	if err := uploadAll(ctx, client, opts, credentials.Uploads, paths); err != nil {
		return nil, err
	}

	thin, err := buildThinArchive(opts.Root, opts.Version)
	if err != nil {
		return nil, err
	}
	defer os.Remove(thin)
	stagedResult, err := stagedput.Put(ctx, stagedput.Options{
		URL: opts.URL, Token: opts.Token, Product: opts.Product, Version: opts.Version,
		File: thin, Single: true, HTTPClient: client, Log: opts.Log,
	})
	if err != nil {
		return nil, err
	}
	return &Result{
		Uploaded:     len(credentials.Uploads),
		Skipped:      len(request.Blobs) - len(credentials.Uploads),
		StagedSHA256: stagedResult.SHA256,
		StagedBytes:  stagedResult.Bytes,
	}, nil
}

func fetchCredentials(ctx context.Context, client *http.Client, opts Options, input credentialRequest) (*credentialResponse, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeBase(opts.URL)+"/cas/credentials", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+opts.Token)
	req.Header.Set("Content-Type", "application/json")
	publishproto.Apply(req.Header)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUpgradeRequired {
		return nil, publishproto.Explain(resp.StatusCode, data)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST cas/credentials HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out credentialResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type blobProgress struct {
	item      upload
	source    string
	etags     []string
	got       int
	completed bool
	mu        sync.Mutex
	abortOnce sync.Once
}

type transfer struct {
	item   upload
	source string
	index  int
	blob   *blobProgress
}

func uploadAll(ctx context.Context, client *http.Client, opts Options, uploads []upload, paths map[string]string) error {
	concurrency := opts.Concurrency
	if concurrency < 1 {
		concurrency = 4
	}
	var multipart []*blobProgress
	var retErr error
	defer func() {
		if retErr == nil {
			return
		}
		abortCtx := context.WithoutCancel(ctx)
		for _, progress := range multipart {
			progress.mu.Lock()
			done := progress.completed
			progress.mu.Unlock()
			if done {
				continue
			}
			progress.abortOnce.Do(func() {
				_ = postAgent(abortCtx, client, opts, "/cas/abort", map[string]string{
					"product":  opts.Product,
					"sha256":   progress.item.SHA256,
					"uploadId": progress.item.UploadID,
				})
			})
		}
	}()
	jobs := make([]transfer, 0)
	for _, item := range uploads {
		normalized, err := normalizeUpload(item)
		if err != nil {
			if item.UploadID != "" {
				multipart = append(multipart, &blobProgress{item: item})
			}
			retErr = err
			return err
		}
		source := paths[strings.ToLower(normalized.SHA256)]
		if source == "" {
			err = fmt.Errorf("credentials returned unknown blob %s", normalized.SHA256)
			if normalized.UploadID != "" {
				multipart = append(multipart, &blobProgress{item: normalized})
			}
			retErr = err
			return err
		}
		if normalized.UploadID == "" {
			jobs = append(jobs, transfer{item: normalized, source: source})
			continue
		}
		progress := &blobProgress{
			item:   normalized,
			source: source,
			etags:  make([]string, len(normalized.Requests)),
		}
		multipart = append(multipart, progress)
		for part := range normalized.Requests {
			jobs = append(jobs, transfer{item: normalized, source: source, index: part, blob: progress})
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	putClient := *client
	putClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	queue := make(chan transfer)
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range queue {
				if err := runTransfer(ctx, &putClient, opts, job); err != nil {
					select {
					case errs <- err:
					default:
					}
					cancel()
					return
				}
			}
		}()
	}
send:
	for _, job := range jobs {
		select {
		case queue <- job:
		case <-ctx.Done():
			break send
		}
	}
	close(queue)
	wg.Wait()
	select {
	case err := <-errs:
		retErr = err
		return err
	default:
		retErr = ctx.Err()
		return ctx.Err()
	}
}

func runTransfer(ctx context.Context, client *http.Client, opts Options, job transfer) error {
	if job.blob == nil {
		if err := uploadOne(ctx, client, job.source, job.item, opts.Log); err != nil {
			return err
		}
		if opts.Log != nil {
			opts.Log(fmt.Sprintf("uploaded cas/%s (%d bytes)", job.item.SHA256, job.item.Size))
		}
		return nil
	}
	etag, err := uploadPart(ctx, client, job.source, job.item, job.index, opts.Log)
	if err != nil {
		return err
	}
	job.blob.mu.Lock()
	job.blob.etags[job.index] = etag
	job.blob.got++
	finished := job.blob.got == len(job.blob.etags)
	etags := append([]string(nil), job.blob.etags...)
	job.blob.mu.Unlock()
	if !finished {
		return nil
	}
	if err := completeBlob(ctx, client, opts, job.item, etags); err != nil {
		return err
	}
	job.blob.mu.Lock()
	job.blob.completed = true
	job.blob.mu.Unlock()
	if opts.Log != nil {
		opts.Log(fmt.Sprintf("uploaded cas/%s (%d bytes)", job.item.SHA256, job.item.Size))
	}
	return nil
}

func normalizeUpload(item upload) (upload, error) {
	if len(item.Requests) == 0 {
		return item, fmt.Errorf("CAS credentials for %s have no requests", item.SHA256)
	}
	if item.UploadID == "" && len(item.Requests) != 1 {
		return item, fmt.Errorf("CAS credentials for %s contain %d requests; this client supports one-request uploads", item.SHA256, len(item.Requests))
	}
	var covered int64
	for i := range item.Requests {
		req := item.Requests[i]
		if req.Method != "" && !strings.EqualFold(req.Method, http.MethodPut) {
			return item, fmt.Errorf("unsupported CAS method %q", req.Method)
		}
		target, err := url.Parse(strings.TrimSpace(req.URL))
		if err != nil {
			return item, err
		}
		if !target.IsAbs() || (target.Scheme != "http" && target.Scheme != "https") {
			return item, fmt.Errorf("CAS upload URL must be an absolute http(s) URL")
		}
		item.Requests[i].URL = target.String()
		item.Requests[i].Method = http.MethodPut
		if item.UploadID == "" {
			continue
		}
		if req.Length < 1 {
			return item, fmt.Errorf("CAS part for %s is missing a length", item.SHA256)
		}
		if req.Offset != covered {
			return item, fmt.Errorf("CAS parts for %s must cover the blob in order", item.SHA256)
		}
		covered += req.Length
	}
	if item.UploadID != "" && covered != item.Size {
		return item, fmt.Errorf("CAS parts for %s cover %d bytes, want %d", item.SHA256, covered, item.Size)
	}
	return item, nil
}

func uploadOne(ctx context.Context, client *http.Client, source string, item upload, log func(string)) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.Size() != item.Size {
		return fmt.Errorf("%s size changed: got %d want %d", source, info.Size(), item.Size)
	}
	var lastErr error
	attemptsUsed := 0
	for attempt := 1; attempt <= maxUploadAttempts; attempt++ {
		attemptsUsed = attempt
		retry, err := uploadOnce(ctx, client, source, item)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry || attempt == maxUploadAttempts {
			break
		}
		delay := time.Duration(1<<(attempt-1)) * time.Second
		if log != nil {
			log(fmt.Sprintf(
				"retrying cas/%s after attempt %d/%d: %v",
				item.SHA256, attempt, maxUploadAttempts, err,
			))
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if attemptsUsed == 1 {
		return lastErr
	}
	return fmt.Errorf("%w (after %d attempts)", lastErr, attemptsUsed)
}

func uploadOnce(ctx context.Context, client *http.Client, source string, item upload) (bool, error) {
	file, err := os.Open(source)
	if err != nil {
		return false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() != item.Size {
		return false, fmt.Errorf("%s size changed: got %d want %d", source, info.Size(), item.Size)
	}
	reqDesc := item.Requests[0]
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqDesc.URL, file)
	if err != nil {
		return false, err
	}
	req.ContentLength = item.Size
	for key, value := range reqDesc.Headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return true, fmt.Errorf("PUT cas/%s: %v", item.SHA256, redactErr(err))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(data))
		return retryableUploadResponse(resp.StatusCode, message),
			fmt.Errorf("PUT cas/%s HTTP %d: %s", item.SHA256, resp.StatusCode, message)
	}
	return false, nil
}

func uploadPart(ctx context.Context, client *http.Client, source string, item upload, index int, log func(string)) (string, error) {
	part := item.Requests[index]
	var lastErr error
	attemptsUsed := 0
	for attempt := 1; attempt <= maxUploadAttempts; attempt++ {
		attemptsUsed = attempt
		etag, retry, err := uploadPartOnce(ctx, client, source, item.SHA256, part)
		if err == nil {
			return etag, nil
		}
		lastErr = err
		if !retry || attempt == maxUploadAttempts {
			break
		}
		delay := time.Duration(1<<(attempt-1)) * time.Second
		if log != nil {
			log(fmt.Sprintf(
				"retrying cas/%s part %d after attempt %d/%d: %v",
				item.SHA256, index+1, attempt, maxUploadAttempts, err,
			))
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	if attemptsUsed == 1 {
		return "", lastErr
	}
	return "", fmt.Errorf("%w (after %d attempts)", lastErr, attemptsUsed)
}

func uploadPartOnce(ctx context.Context, client *http.Client, source, digest string, part uploadRequest) (string, bool, error) {
	file, err := os.Open(source)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	if _, err := file.Seek(part.Offset, io.SeekStart); err != nil {
		return "", false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, part.URL, io.LimitReader(file, part.Length))
	if err != nil {
		return "", false, err
	}
	req.ContentLength = part.Length
	for key, value := range part.Headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		return "", true, fmt.Errorf("PUT cas/%s: %v", digest, redactErr(err))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(data))
		return "", retryableUploadResponse(resp.StatusCode, message),
			fmt.Errorf("PUT cas/%s HTTP %d: %s", digest, resp.StatusCode, message)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", false, fmt.Errorf("PUT cas/%s part returned no ETag", digest)
	}
	return etag, false, nil
}

func completeBlob(ctx context.Context, client *http.Client, opts Options, item upload, etags []string) error {
	parts := make([]partReport, len(etags))
	for i, etag := range etags {
		parts[i] = partReport{PartNumber: i + 1, ETag: etag}
	}
	return postAgent(ctx, client, opts, "/cas/complete", map[string]any{
		"product":  opts.Product,
		"sha256":   item.SHA256,
		"uploadId": item.UploadID,
		"parts":    parts,
	})
}

func postAgent(ctx context.Context, client *http.Client, opts Options, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeBase(opts.URL)+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+opts.Token)
	req.Header.Set("Content-Type", "application/json")
	publishproto.Apply(req.Header)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("POST %s HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func retryableUploadResponse(status int, body string) bool {
	if status == http.StatusRequestTimeout ||
		status == http.StatusTooEarly ||
		status == http.StatusTooManyRequests ||
		status >= http.StatusInternalServerError {
		return true
	}
	// Tencent COS reports a transport-speed timeout as HTTP 400 even though
	// replaying the same idempotent, content-addressed PUT is safe.
	return status == http.StatusBadRequest && strings.Contains(body, "<Code>UserNetworkTooSlow</Code>")
}

func redactErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", httpx.RedactText(err.Error()))
}

func buildThinArchive(root, version string) (string, error) {
	file, err := os.CreateTemp("", "relkit-staged-thin-*.tar.gz")
	if err != nil {
		return "", err
	}
	target := file.Name()
	cleanup := func(err error) (string, error) {
		_ = file.Close()
		_ = os.Remove(target)
		return "", err
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for _, source := range []string{stage.StagedPath(root, version), stage.ReleasePolicyPath(root, version)} {
		info, err := os.Stat(source)
		if err != nil {
			return cleanup(err)
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return cleanup(err)
		}
		header.Name = filepath.Base(source)
		if err := tw.WriteHeader(header); err != nil {
			return cleanup(err)
		}
		input, err := os.Open(source)
		if err != nil {
			return cleanup(err)
		}
		_, copyErr := io.Copy(tw, input)
		closeErr := input.Close()
		if copyErr != nil {
			return cleanup(copyErr)
		}
		if closeErr != nil {
			return cleanup(closeErr)
		}
	}
	if err := tw.Close(); err != nil {
		return cleanup(err)
	}
	if err := gz.Close(); err != nil {
		return cleanup(err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(target)
		return "", err
	}
	return target, nil
}

func normalizeBase(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if strings.HasSuffix(base, "/v1") {
		return base
	}
	return base + "/v1"
}
