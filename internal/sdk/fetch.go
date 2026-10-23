package sdk

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Progress reports download progress.
type Progress struct {
	Received      int64
	Total         int64
	BytesPerSecond int64
}

// ProgressFunc is called during downloads.
type ProgressFunc func(Progress)

// ResourceProbe is the result of a Range capability probe.
type ResourceProbe struct {
	AcceptRanges bool
	ContentLength int64
	StatusCode   int
}

// Fetcher abstracts HTTP for tests and hosts that need custom clients.
type Fetcher interface {
	GetBytes(ctx context.Context, rawURL string) ([]byte, error)
	Probe(ctx context.Context, rawURL string) (ResourceProbe, error)
	Download(ctx context.Context, rawURL string, startOffset int64, w io.Writer, onProgress ProgressFunc) (int64, error)
	DownloadRange(ctx context.Context, rawURL string, start, end int64, w io.Writer) (int64, error)
}

// HTTPFetcher is the default Fetcher.
type HTTPFetcher struct {
	Client          *http.Client
	DocumentTimeout time.Duration
	IdleTimeout     time.Duration
	MaxRedirects    int
}

func (f *HTTPFetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{
		Timeout: 0,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			max := f.MaxRedirects
			if max <= 0 {
				max = 5
			}
			if len(via) >= max {
				return fmt.Errorf("stopped after %d redirects", max)
			}
			if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme == "http" {
				return fmt.Errorf("refusing https to http redirect")
			}
			return nil
		},
	}
}

func (f *HTTPFetcher) documentTimeout() time.Duration {
	if f.DocumentTimeout > 0 {
		return f.DocumentTimeout
	}
	return 10 * time.Second
}

// GetBytes fetches a whole document body (capped at 32 MiB).
func (f *HTTPFetcher) GetBytes(ctx context.Context, rawURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, f.documentTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

// Probe issues a ranged GET to learn Content-Length and Accept-Ranges.
func (f *HTTPFetcher) Probe(ctx context.Context, rawURL string) (ResourceProbe, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ResourceProbe{}, err
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := f.client().Do(req)
	if err != nil {
		return ResourceProbe{}, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64))
	probe := ResourceProbe{StatusCode: resp.StatusCode}
	if resp.StatusCode == http.StatusPartialContent {
		probe.AcceptRanges = true
		if cr := resp.Header.Get("Content-Range"); cr != "" {
			if i := strings.LastIndex(cr, "/"); i >= 0 {
				if n, err := strconv.ParseInt(cr[i+1:], 10, 64); err == nil {
					probe.ContentLength = n
				}
			}
		}
	} else if cl := resp.Header.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil {
			probe.ContentLength = n
		}
	}
	if strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes") {
		probe.AcceptRanges = true
	}
	return probe, nil
}

// Download streams from startOffset to w.
func (f *HTTPFetcher) Download(ctx context.Context, rawURL string, startOffset int64, w io.Writer, onProgress ProgressFunc) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	if startOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if startOffset > 0 && resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("server refused Range (HTTP %d)", resp.StatusCode)
	}
	if startOffset == 0 && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var total int64
	if cl := resp.ContentLength; cl >= 0 {
		total = startOffset + cl
	}
	return copyWithProgress(resp.Body, w, startOffset, total, onProgress)
}

// DownloadRange fetches an inclusive byte range.
func (f *HTTPFetcher) DownloadRange(ctx context.Context, rawURL string, start, end int64, w io.Writer) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	resp, err := f.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.Copy(w, resp.Body)
}

func copyWithProgress(r io.Reader, w io.Writer, already, total int64, onProgress ProgressFunc) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	start := time.Now()
	lastReport := start
	for {
		n, err := r.Read(buf)
		if n > 0 {
			nw, werr := w.Write(buf[:n])
			written += int64(nw)
			if werr != nil {
				return written, werr
			}
			if onProgress != nil && time.Since(lastReport) >= 200*time.Millisecond {
				elapsed := time.Since(start).Seconds()
				bps := int64(0)
				if elapsed > 0 {
					bps = int64(float64(written) / elapsed)
				}
				onProgress(Progress{Received: already + written, Total: total, BytesPerSecond: bps})
				lastReport = time.Now()
			}
		}
		if err == io.EOF {
			if onProgress != nil {
				elapsed := time.Since(start).Seconds()
				bps := int64(0)
				if elapsed > 0 {
					bps = int64(float64(written) / elapsed)
				}
				onProgress(Progress{Received: already + written, Total: total, BytesPerSecond: bps})
			}
			return written, nil
		}
		if err != nil {
			return written, err
		}
	}
}
