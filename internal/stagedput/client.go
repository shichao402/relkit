package stagedput

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"cnb.cool/shichao402/relkit/internal/humansize"
)

const (
	DefaultPartSize    int64 = 8 << 20
	DefaultConcurrency       = 8
	partSHAHeader            = "X-Relkit-Part-SHA256"
)

type Options struct {
	URL         string
	Token       string
	Product     string
	Version     string
	File        string
	PartSize    int64
	Concurrency int
	Single      bool
	HTTPClient  *http.Client
	Log         func(string)
}

type Result struct {
	SHA256 string
	Bytes  int64
}

type Session struct {
	ID             string `json:"id"`
	Product        string `json:"product"`
	Version        string `json:"version"`
	Bytes          int64  `json:"bytes"`
	SHA256         string `json:"sha256"`
	PartSize       int64  `json:"partSize"`
	PartCount      int    `json:"partCount"`
	MaxConcurrency int    `json:"maxConcurrency"`
	Status         string `json:"status"`
	Received       []int  `json:"received"`
}

func Put(ctx context.Context, opts Options) (*Result, error) {
	if err := validate(opts); err != nil {
		return nil, err
	}
	base := normalizeBase(opts.URL)
	info, err := os.Stat(opts.File)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", opts.File)
	}
	sum, err := hashFile(opts.File)
	if err != nil {
		return nil, err
	}
	client := opts.HTTPClient
	if client == nil {
		concurrency := opts.Concurrency
		if concurrency < 1 {
			concurrency = DefaultConcurrency
		}
		client = &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        64,
				MaxIdleConnsPerHost: concurrency + 2,
				MaxConnsPerHost:     concurrency + 2,
			},
		}
	}
	c := &runner{opts: opts, base: base, client: client, bytes: info.Size(), sha256: sum}
	if opts.Single {
		return c.putWhole(ctx)
	}
	return c.putParts(ctx)
}

func DefaultPartSizeFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv("RELKIT_UPLOAD_PART_SIZE"))
	if raw == "" {
		return DefaultPartSize
	}
	n, err := humansize.Parse(raw)
	if err != nil || n < 1 {
		return DefaultPartSize
	}
	return n
}

func DefaultConcurrencyFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("RELKIT_UPLOAD_CONCURRENCY"))
	if raw == "" {
		return DefaultConcurrency
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return DefaultConcurrency
	}
	return n
}

type runner struct {
	opts   Options
	base   string
	client *http.Client
	bytes  int64
	sha256 string
}

func (c *runner) log(format string, args ...any) {
	if c.opts.Log == nil {
		return
	}
	c.opts.Log(fmt.Sprintf(format, args...))
}

func (c *runner) putWhole(ctx context.Context) (*Result, error) {
	f, err := os.Open(c.opts.File)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.stagedURL(), f)
	if err != nil {
		return nil, err
	}
	req.ContentLength = c.bytes
	req.Header.Set("Authorization", "Bearer "+c.opts.Token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("PUT staged HTTP %d: %s", resp.StatusCode, trimBody(body))
	}
	c.log("uploaded %d bytes in one request", c.bytes)
	return &Result{SHA256: c.sha256, Bytes: c.bytes}, nil
}

func (c *runner) putParts(ctx context.Context) (*Result, error) {
	session, err := c.create(ctx)
	if err != nil {
		return nil, err
	}
	concurrency := c.opts.Concurrency
	if concurrency < 1 {
		concurrency = DefaultConcurrency
	}
	if session.MaxConcurrency > 0 && concurrency > session.MaxConcurrency {
		concurrency = session.MaxConcurrency
	}
	c.log("upload %s: %d bytes in %d parts of %d bytes, concurrency %d", session.ID, session.Bytes, session.PartCount, session.PartSize, concurrency)

	pending := pendingParts(session)
	if err := c.uploadPending(ctx, session, pending, concurrency); err != nil {
		return nil, err
	}
	if err := c.complete(ctx, session.ID); err != nil {
		session, getErr := c.get(ctx, session.ID)
		if getErr == nil {
			pending = pendingParts(session)
			if len(pending) > 0 {
				if err := c.uploadPending(ctx, session, pending, concurrency); err != nil {
					return nil, err
				}
				if err := c.complete(ctx, session.ID); err != nil {
					return nil, err
				}
				return &Result{SHA256: c.sha256, Bytes: c.bytes}, nil
			}
		}
		return nil, err
	}
	return &Result{SHA256: c.sha256, Bytes: c.bytes}, nil
}

func (c *runner) uploadPending(ctx context.Context, session *Session, pending []int, concurrency int) error {
	if len(pending) == 0 {
		return nil
	}
	if concurrency < 1 {
		concurrency = 1
	}
	jobs := make(chan int, len(pending))
	for _, part := range pending {
		jobs <- part
	}
	close(jobs)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	worker := func() {
		defer wg.Done()
		for part := range jobs {
			mu.Lock()
			failed := first != nil
			mu.Unlock()
			if failed || ctx.Err() != nil {
				return
			}
			if err := c.putPart(ctx, session, part); err != nil {
				mu.Lock()
				if first == nil {
					first = err
				}
				mu.Unlock()
				return
			}
		}
	}
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go worker()
	}
	wg.Wait()
	return first
}

func (c *runner) create(ctx context.Context) (*Session, error) {
	payload := map[string]any{
		"bytes":  c.bytes,
		"sha256": c.sha256,
	}
	if c.opts.PartSize > 0 {
		payload["partSize"] = c.opts.PartSize
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.stagedURL()+"/uploads", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.opts.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create upload HTTP %d: %s", resp.StatusCode, trimBody(body))
	}
	var session Session
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, fmt.Errorf("create upload: %w", err)
	}
	if session.ID == "" || session.PartCount < 1 || session.PartSize < 1 {
		return nil, fmt.Errorf("create upload: incomplete session")
	}
	return &session, nil
}

func (c *runner) get(ctx context.Context, id string) (*Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.stagedURL()+"/uploads/"+id, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.opts.Token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get upload HTTP %d: %s", resp.StatusCode, trimBody(body))
	}
	var session Session
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *runner) complete(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.stagedURL()+"/uploads/"+id+"/complete", http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.opts.Token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("complete upload HTTP %d: %s", resp.StatusCode, trimBody(body))
	}
	return nil
}

func (c *runner) putPart(ctx context.Context, session *Session, part int) error {
	start := session.PartSize * int64(part)
	length := session.PartSize
	if part == session.PartCount-1 {
		length = session.Bytes - start
	}
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt*attempt) * 200 * time.Millisecond
			c.log("part %d retry %d after %s: %v", part, attempt, delay, last)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		err := c.putPartOnce(ctx, session.ID, part, start, length)
		if err == nil {
			return nil
		}
		last = err
		if !retryable(err) {
			return err
		}
	}
	return last
}

func (c *runner) putPartOnce(ctx context.Context, id string, part int, start, length int64) error {
	f, err := os.Open(c.opts.File)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(h, io.NewSectionReader(f, start, length)); err != nil {
		return err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	section := io.NewSectionReader(f, start, length)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/uploads/%s/parts/%d", c.stagedURL(), id, part),
		section,
	)
	if err != nil {
		return err
	}
	req.ContentLength = length
	req.Header.Set("Authorization", "Bearer "+c.opts.Token)
	req.Header.Set(partSHAHeader, sum)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, trimBody(body))
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("part %d HTTP %d: %s", part, resp.StatusCode, trimBody(body))
	}
	return nil
}

func (c *runner) stagedURL() string {
	return strings.TrimRight(c.base, "/") + "/staged/" + path.Join(c.opts.Product, c.opts.Version)
}

func pendingParts(session *Session) []int {
	have := map[int]bool{}
	for _, n := range session.Received {
		have[n] = true
	}
	var pending []int
	for i := 0; i < session.PartCount; i++ {
		if !have[i] {
			pending = append(pending, i)
		}
	}
	return pending
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func validate(opts Options) error {
	if strings.TrimSpace(opts.Product) == "" || strings.TrimSpace(opts.Version) == "" {
		return fmt.Errorf("product and version are required")
	}
	if strings.TrimSpace(opts.URL) == "" {
		return fmt.Errorf("agent URL is required")
	}
	if strings.TrimSpace(opts.Token) == "" {
		return fmt.Errorf("upload token is required")
	}
	if strings.TrimSpace(opts.File) == "" {
		return fmt.Errorf("file is required")
	}
	return nil
}

func normalizeBase(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if strings.HasSuffix(base, "/v1") {
		return base
	}
	return base + "/v1"
}

func trimBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 400 {
		return s[:400]
	}
	return s
}

func retryable(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "HTTP 429") || strings.Contains(msg, "HTTP 5") || strings.Contains(msg, "connection")
}
