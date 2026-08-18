package sdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
)

// VerifiedFile is a downloaded artifact that passed size and sha256 checks.
type VerifiedFile struct {
	Path      string
	Artifact  *rupv2.Artifact
	SourceURL string
}

type partMeta struct {
	URL    string  `json:"url"`
	Size   int64   `json:"size"`
	Sha256 string  `json:"sha256"`
	Done   [][2]int64 `json:"done,omitempty"`
}

// DownloadArtifact downloads artifact.urls in order with retries, optional
// Range resume / parallel chunks, and sha256 verification.
func DownloadArtifact(ctx context.Context, fetcher Fetcher, artifact *rupv2.Artifact, destPath string, policy Policy, onProgress ProgressFunc) (*VerifiedFile, error) {
	if artifact == nil || len(artifact.Urls) == 0 {
		return nil, fmt.Errorf("nil artifact or empty urls")
	}
	if fetcher == nil {
		fetcher = &HTTPFetcher{}
	}
	if policy.DownloadRetries <= 0 {
		policy.DownloadRetries = 3
	}
	if policy.DownloadWorkers <= 0 {
		policy.DownloadWorkers = 8
	}
	if policy.DownloadChunkSize <= 0 {
		policy.DownloadChunkSize = 4 << 20
	}

	if ok, err := fileMatches(destPath, artifact.Sha256, artifact.Size); err == nil && ok {
		return &VerifiedFile{Path: destPath, Artifact: artifact}, nil
	}

	var last error
	for _, url := range artifact.Urls {
		for attempt := 0; attempt < policy.DownloadRetries; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(time.Duration(1<<uint(attempt-1)) * 200 * time.Millisecond):
				}
			}
			if err := downloadOne(ctx, fetcher, url, artifact, destPath, policy, onProgress); err != nil {
				last = err
				_ = os.Remove(destPath + ".part")
				_ = os.Remove(destPath + ".part.meta")
				continue
			}
			return &VerifiedFile{Path: destPath, Artifact: artifact, SourceURL: url}, nil
		}
	}
	if last == nil {
		last = fmt.Errorf("all artifact mirrors failed")
	}
	return nil, last
}

func downloadOne(ctx context.Context, fetcher Fetcher, url string, artifact *rupv2.Artifact, destPath string, policy Policy, onProgress ProgressFunc) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil && !os.IsExist(err) {
		// dest may be in cwd with no parent
	}
	partPath := destPath + ".part"
	metaPath := destPath + ".part.meta"

	probe, _ := fetcher.Probe(ctx, url)
	if probe.AcceptRanges && probe.ContentLength == artifact.Size && policy.DownloadWorkers > 1 && artifact.Size > policy.DownloadChunkSize {
		if err := downloadParallel(ctx, fetcher, url, artifact, partPath, metaPath, policy, onProgress); err != nil {
			return err
		}
	} else {
		start := int64(0)
		if st, err := os.Stat(partPath); err == nil {
			if meta, merr := loadPartMeta(metaPath); merr == nil && meta.URL == url && meta.Sha256 == artifact.Sha256 && meta.Size == artifact.Size {
				start = st.Size()
				if start > artifact.Size {
					start = 0
					_ = os.Remove(partPath)
				}
			} else {
				_ = os.Remove(partPath)
			}
		}
		f, err := os.OpenFile(partPath, os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			return err
		}
		if start > 0 {
			if _, err := f.Seek(start, io.SeekStart); err != nil {
				f.Close()
				return err
			}
		} else {
			if err := f.Truncate(0); err != nil {
				f.Close()
				return err
			}
		}
		_, err = fetcher.Download(ctx, url, start, f, onProgress)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		_ = writePartMeta(metaPath, partMeta{URL: url, Size: artifact.Size, Sha256: artifact.Sha256})
	}

	if ok, err := fileMatches(partPath, artifact.Sha256, artifact.Size); err != nil {
		return err
	} else if !ok {
		_ = os.Remove(partPath)
		_ = os.Remove(metaPath)
		return fmt.Errorf("sha256 or size mismatch")
	}
	_ = os.Remove(destPath)
	if err := os.Rename(partPath, destPath); err != nil {
		return err
	}
	_ = os.Remove(metaPath)
	return nil
}

func downloadParallel(ctx context.Context, fetcher Fetcher, url string, artifact *rupv2.Artifact, partPath, metaPath string, policy Policy, onProgress ProgressFunc) error {
	f, err := os.OpenFile(partPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(artifact.Size); err != nil {
		return err
	}

	type chunk struct{ start, end int64 }
	var chunks []chunk
	for start := int64(0); start < artifact.Size; start += policy.DownloadChunkSize {
		end := start + policy.DownloadChunkSize - 1
		if end >= artifact.Size {
			end = artifact.Size - 1
		}
		chunks = append(chunks, chunk{start, end})
	}

	meta, _ := loadPartMeta(metaPath)
	done := map[[2]int64]bool{}
	if meta.URL == url && meta.Sha256 == artifact.Sha256 {
		for _, d := range meta.Done {
			done[[2]int64{d[0], d[1]}] = true
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	sem := make(chan struct{}, policy.DownloadWorkers)
	var received int64
	for _, d := range meta.Done {
		received += d[1] - d[0] + 1
	}

	for _, c := range chunks {
		c := c
		if done[[2]int64{c.start, c.end}] {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			var buf writerAtBuf
			buf.buf = make([]byte, c.end-c.start+1)
			n, err := fetcher.DownloadRange(ctx, url, c.start, c.end, &buf)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			if n != int64(len(buf.buf)) {
				select {
				case errCh <- fmt.Errorf("short range read"):
				default:
				}
				return
			}
			mu.Lock()
			_, err = f.WriteAt(buf.buf, c.start)
			if err == nil {
				received += n
				meta.URL = url
				meta.Size = artifact.Size
				meta.Sha256 = artifact.Sha256
				meta.Done = append(meta.Done, [2]int64{c.start, c.end})
				_ = writePartMeta(metaPath, meta)
				if onProgress != nil {
					onProgress(Progress{Received: received, Total: artifact.Size})
				}
			}
			mu.Unlock()
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

type writerAtBuf struct {
	buf []byte
	off int
}

func (w *writerAtBuf) Write(p []byte) (int, error) {
	n := copy(w.buf[w.off:], p)
	w.off += n
	if n < len(p) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

func fileMatches(path, wantSHA string, wantSize int64) (bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if st.Size() != wantSize {
		return false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == wantSHA, nil
}

func loadPartMeta(path string) (partMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return partMeta{}, err
	}
	var m partMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return partMeta{}, err
	}
	return m, nil
}

func writePartMeta(path string, m partMeta) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
