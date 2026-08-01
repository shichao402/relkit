package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Benchmarks run over loopback against a real listener, so they measure the
// HTTP stack and the file path rather than the network. That is the useful
// question here: whether the server is the bottleneck. On a real deployment the
// limit moves to the disk and the NIC, both of which these numbers dwarf.
//
//	go test -bench . -benchtime 3s

func benchServer(b *testing.B, sizeMB int) (string, func()) {
	b.Helper()
	dir := b.TempDir()

	name := filepath.Join(dir, "artifact")
	if err := os.MkdirAll(name, 0o755); err != nil {
		b.Fatal(err)
	}
	file := filepath.Join(name, "big.zip")
	body := payload(sizeMB << 20)
	if err := os.WriteFile(file, body, 0o644); err != nil {
		b.Fatal(err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		b.Fatal(err)
	}
	cfg := &config{
		root:          root,
		rootPath:      dir,
		immutable:     []string{"artifact/"},
		defaultMaxAge: 60,
	}

	srv := newLocalServer(b, cfg)
	return srv.URL + "/artifact/big.zip", func() {
		srv.Close()
		root.Close()
	}
}

func BenchmarkWholeFile(b *testing.B) {
	const sizeMB = 32
	url, done := benchServer(b, sizeMB)
	defer done()

	b.SetBytes(int64(sizeMB) << 20)
	b.ResetTimer()
	for b.Loop() {
		resp, err := http.Get(url)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}

// BenchmarkParallelWholeFile approximates a fleet updating at once: many
// independent clients each pulling a full release.
//
// Concurrency is driven by an explicit worker count sharing one connection pool
// rather than by RunParallel. RunParallel's parallelism is per-P, so asking for
// 256 on a 32-thread machine opens 8192 connections and exhausts the ephemeral
// port range -- measuring the test harness rather than the server.
func BenchmarkParallelWholeFile(b *testing.B) {
	const sizeMB = 16
	url, done := benchServer(b, sizeMB)
	defer done()

	for _, clients := range []int{8, 64, 256} {
		b.Run(fmt.Sprintf("clients=%d", clients), func(b *testing.B) {
			client := &http.Client{Transport: &http.Transport{
				MaxIdleConns:        clients,
				MaxIdleConnsPerHost: clients,
				DisableCompression:  true,
			}}

			// Retries cover the accept queue filling up during the initial
			// connection burst. A real fleet of this size does not connect
			// within the same microsecond, and real clients retry; without this
			// the benchmark measures the listen backlog instead of throughput.
			fetch := func() error {
				var err error
				for attempt := range 3 {
					var resp *http.Response
					resp, err = client.Get(url)
					if err != nil {
						time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
						continue
					}
					_, err = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					return err
				}
				return err
			}

			if err := fetch(); err != nil {
				b.Fatal(err)
			}

			b.SetBytes(int64(sizeMB) << 20)
			b.ResetTimer()

			var issued atomic.Int64
			var wg sync.WaitGroup
			for i := range clients {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					// Stagger the ramp-up for the same reason.
					time.Sleep(time.Duration(i) * 200 * time.Microsecond)
					for issued.Add(1) <= int64(b.N) {
						if err := fetch(); err != nil {
							b.Error(err)
							return
						}
					}
				}(i)
			}
			wg.Wait()
		})
	}
}

// BenchmarkRangedSlices is one client downloading with many threads: the same
// bytes, but split across concurrent ranged requests.
func BenchmarkRangedSlices(b *testing.B) {
	const sizeMB = 16
	const slices = 16
	url, done := benchServer(b, sizeMB)
	defer done()

	total := int64(sizeMB) << 20
	step := total / slices

	b.SetBytes(total)
	b.ResetTimer()
	for b.Loop() {
		done := make(chan error, slices)
		for i := range slices {
			go func(i int) {
				start := int64(i) * step
				end := start + step - 1
				if i == slices-1 {
					end = total - 1
				}
				req, _ := http.NewRequest("GET", url, nil)
				req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					done <- err
					return
				}
				_, err = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				done <- err
			}(i)
		}
		for range slices {
			if err := <-done; err != nil {
				b.Fatal(err)
			}
		}
	}
}
