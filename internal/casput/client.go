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

	"cnb.cool/shichao402/relkit/internal/backends"
	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/stage"
	"cnb.cool/shichao402/relkit/internal/stagedput"
)

type Options struct {
	Root        string
	Product     string
	Version     string
	URL         string
	Token       string
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
	Product string `json:"product"`
	Blobs   []blob `json:"blobs"`
}

type upload struct {
	SHA256  string            `json:"sha256"`
	Size    int64             `json:"size"`
	PutURL  string            `json:"putUrl"`
	Headers map[string]string `json:"headers,omitempty"`
	Sign    *backends.CASSign `json:"sign,omitempty"`
}

type credentialResponse struct {
	ExpiresAt time.Time `json:"expiresAt"`
	Uploads   []upload  `json:"uploads"`
}

func Put(ctx context.Context, opts Options) (*Result, error) {
	if opts.Root == "" || opts.Product == "" || opts.Version == "" || opts.URL == "" || opts.Token == "" {
		return nil, fmt.Errorf("root, product, version, url, and token are required")
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
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Hour}
	}
	request := credentialRequest{Product: opts.Product}
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
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST cas/credentials HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out credentialResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func uploadAll(ctx context.Context, client *http.Client, opts Options, uploads []upload, paths map[string]string) error {
	concurrency := opts.Concurrency
	if concurrency < 1 {
		concurrency = 4
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	putClient := *client
	putClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	jobs := make(chan upload)
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				resolved, err := resolveUploadURL(opts.URL, item.PutURL)
				if err != nil {
					select {
					case errs <- err:
					default:
					}
					cancel()
					return
				}
				item.PutURL = resolved
				source := paths[strings.ToLower(item.SHA256)]
				if source == "" {
					select {
					case errs <- fmt.Errorf("credentials returned unknown blob %s", item.SHA256):
					default:
					}
					cancel()
					return
				}
				if err := uploadOne(ctx, &putClient, source, item); err != nil {
					select {
					case errs <- err:
					default:
					}
					cancel()
					return
				}
				if opts.Log != nil {
					opts.Log(fmt.Sprintf("uploaded cas/%s (%d bytes)", item.SHA256, item.Size))
				}
			}
		}()
	}
send:
	for _, item := range uploads {
		select {
		case jobs <- item:
		case <-ctx.Done():
			break send
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errs:
		return err
	default:
		return ctx.Err()
	}
}

func resolveUploadURL(agentURL, putURL string) (string, error) {
	target, err := url.Parse(strings.TrimSpace(putURL))
	if err != nil {
		return "", err
	}
	if target.IsAbs() {
		if target.Scheme != "http" && target.Scheme != "https" {
			return "", fmt.Errorf("unsupported CAS upload URL scheme %q", target.Scheme)
		}
		return target.String(), nil
	}
	base, err := url.Parse(strings.TrimSpace(agentURL))
	if err != nil || !base.IsAbs() {
		return "", fmt.Errorf("cannot resolve relative CAS upload URL against %q", agentURL)
	}
	base.Path = "/"
	base.RawQuery = ""
	base.Fragment = ""
	return base.ResolveReference(target).String(), nil
}

func uploadOne(ctx context.Context, client *http.Client, source string, item upload) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() != item.Size {
		return fmt.Errorf("%s size changed: got %d want %d", source, info.Size(), item.Size)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, item.PutURL, file)
	if err != nil {
		return err
	}
	req.ContentLength = item.Size
	for key, value := range item.Headers {
		req.Header.Set(key, value)
	}
	if err := backends.ApplyCASSign(req, item.Sign, time.Now()); err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("PUT cas/%s HTTP %d: %s", item.SHA256, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
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
