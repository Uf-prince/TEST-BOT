package goldcmds

// ============================================================================
// GOLD-MD — Fast ranged media downloader
// File: fastdl.go
// ============================================================================
// googlevideo throttles any single request that pulls more than ~8 MiB down to
// roughly 0.7 MB/s, so a 77 MB 1080p stream used to need ~110s and the 2-minute
// downloader watchdog killed it. The SAME bytes requested as <=8 MiB ranged
// chunks come back at 28+ MB/s each, and chunks can be fetched in parallel —
// the full 77 MB file lands in well under a second.
//
// streamDownloadToFile tries this path first and keeps its sequential
// single-stream loop as the fallback for servers that ignore Range.
// ============================================================================

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	dlChunkSize = 8 << 20 // 8 MiB — largest chunk googlevideo serves unthrottled
	dlWorkers   = 4       // bounded parallelism (extra RAM: workers * 64 KiB)
	dlMaxChunks = 512     // parallelise up to 4 GiB, beyond that stream sequentially
	dlAttempts  = 3       // per-chunk retries
)

// dlZeroProgress reports whether no bytes were written, i.e. it is safe to
// retry the same URL with a different strategy without wasting bandwidth.
func dlZeroProgress(written int64) bool { return written == 0 }

// rangedDownload fetches url as parallel ranged chunks into dst.
//
// It returns supported=false (with a nil error) when the server ignores Range
// and answers 200 with the whole body — the caller must then use the
// sequential path. A non-nil error means the ranged attempt failed; the caller
// may still fall back when dlZeroProgress(downloaded) holds.
func rangedDownload(ctx context.Context, client *http.Client, url string, dst *os.File, progress func(int)) (supported bool, downloaded int64, err error) {
	first, total, err := dlRangeProbe(ctx, client, url)
	if err != nil {
		return false, 0, err
	}
	if first == nil {
		return false, 0, nil // server does not honour Range
	}

	// Whole file fit in the probe request.
	if total <= int64(len(first)) {
		if _, werr := dst.WriteAt(first, 0); werr != nil {
			return true, 0, werr
		}
		if progress != nil {
			progress(100)
		}
		return true, int64(len(first)), nil
	}
	if _, werr := dst.WriteAt(first, 0); werr != nil {
		return true, 0, werr
	}
	var done int64 = int64(len(first))
	if progress != nil {
		progress(int(done * 100 / total))
	}

	nChunks := int((total + dlChunkSize - 1) / dlChunkSize)
	if nChunks > dlMaxChunks {
		// Absurdly large media: keep the bounded, sequential path.
		return false, 0, nil
	}

	// Worker pool over the remaining chunks.
	chunkIdx := make(chan int)
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		firstEr error
	)
	fail := func(e error) {
		mu.Lock()
		if firstEr == nil {
			firstEr = e
			cancel()
		}
		mu.Unlock()
	}

	workers := dlWorkers
	if workers > nChunks-1 {
		workers = nChunks - 1
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64*1024)
			for i := range chunkIdx {
				if wctx.Err() != nil {
					return
				}
				start := int64(i) * dlChunkSize
				end := start + dlChunkSize - 1
				if end > total-1 {
					end = total - 1
				}
				n, cerr := dlFetchRange(wctx, client, url, start, end, dst, buf)
				if cerr != nil {
					fail(cerr)
					return
				}
				cur := atomic.AddInt64(&done, n)
				if progress != nil {
					progress(int(cur * 100 / total))
				}
			}
		}()
	}
	for i := 1; i < nChunks; i++ {
		select {
		case chunkIdx <- i:
		case <-wctx.Done():
			break
		}
		if wctx.Err() != nil {
			break
		}
	}
	close(chunkIdx)
	wg.Wait()

	mu.Lock()
	ferr := firstEr
	mu.Unlock()
	if ferr != nil {
		return true, atomic.LoadInt64(&done), ferr
	}
	if atomic.LoadInt64(&done) != total {
		return true, atomic.LoadInt64(&done), fmt.Errorf("short ranged download: got %d of %d bytes", atomic.LoadInt64(&done), total)
	}
	if progress != nil {
		progress(100)
	}
	return true, total, nil
}

// dlRangeProbe issues the first ranged request and returns the bytes it
// carried plus the full media size parsed from Content-Range. A nil slice with
// a nil error means the server does not honour Range.
func dlRangeProbe(ctx context.Context, client *http.Client, url string) ([]byte, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", dlChunkSize-1))

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		// 200 => Range ignored, body is the whole file: hand off to the
		// sequential loop (drain a little so the connection can be reused).
		_, _ = io.CopyN(io.Discard, resp.Body, 32*1024)
		return nil, 0, nil
	}

	total, ok := dlParseContentRange(resp.Header.Get("Content-Range"))
	if !ok || total <= 0 {
		// Length unknown: a parallel plan cannot be built safely.
		return nil, 0, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, dlChunkSize))
	if err != nil {
		return nil, 0, err
	}
	return body, total, nil
}

// dlFetchRange downloads [start,end] and writes it at the right file offset,
// retrying transient failures. It returns the number of bytes written.
func dlFetchRange(ctx context.Context, client *http.Client, url string, start, end int64, dst *os.File, buf []byte) (int64, error) {
	want := end - start + 1
	var lastErr error
	for attempt := 0; attempt < dlAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			lastErr = fmt.Errorf("range request returned status %d", resp.StatusCode)
			continue
		}
		off := start
		got := int64(0)
		for got < want {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := dst.WriteAt(buf[:n], off); werr != nil {
					resp.Body.Close()
					return got, werr
				}
				off += int64(n)
				got += int64(n)
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				lastErr = rerr
				break
			}
		}
		resp.Body.Close()
		if got == want {
			return got, nil
		}
		lastErr = fmt.Errorf("short chunk %d-%d: got %d of %d", start, end, got, want)
	}
	if lastErr == nil {
		lastErr = errors.New("range download failed")
	}
	return 0, lastErr
}

// dlParseContentRange extracts the total size from a Content-Range header of
// the form "bytes 0-8388607/80911999".
func dlParseContentRange(h string) (int64, bool) {
	i := strings.LastIndex(h, "/")
	if i < 0 {
		return 0, false
	}
	total := strings.TrimSpace(h[i+1:])
	if total == "*" || total == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(total, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
