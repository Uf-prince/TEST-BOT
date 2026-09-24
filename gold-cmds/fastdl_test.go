package goldcmds

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDLParseContentRange(t *testing.T) {
	cases := []struct {
		in    string
		total int64
		ok    bool
	}{
		{"bytes 0-8388607/80911999", 80911999, true},
		{"bytes 0-1/2", 2, true},
		{"bytes 0-100/*", 0, false},
		{"", 0, false},
		{"garbage", 0, false},
	}
	for _, c := range cases {
		got, ok := dlParseContentRange(c.in)
		if ok != c.ok || (ok && got != c.total) {
			t.Errorf("dlParseContentRange(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.total, c.ok)
		}
	}
}

// TestRangedDownloadReassembles is the core regression test: a server that
// honours Range must be fetched in parallel chunks and reassembled byte-exact
// at the correct offsets.
func TestRangedDownloadReassembles(t *testing.T) {
	payload := make([]byte, 3*dlChunkSize+12345) // forces 4 chunks, last one short
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		serveRange(t, w, r, payload)
	}))
	defer srv.Close()

	got, err := downloadViaStream(t, srv.URL)
	if err != nil {
		t.Fatalf("download error: %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("size mismatch: got %d want %d", len(got), len(payload))
	}
	if string(got) != string(payload) {
		t.Fatal("content mismatch: ranged reassembly is not byte-exact")
	}
	if requests < 4 {
		t.Errorf("expected >=4 ranged requests, got %d (parallel chunking not used)", requests)
	}
}

// TestRangedDownloadFallsBackWhenRangeIgnored ensures a server that answers the
// probe with a full 200 body still yields the complete file via the sequential
// path.
func TestRangedDownloadFallsBackWhenRangeIgnored(t *testing.T) {
	payload := []byte(strings.Repeat("gold-md-fallback-payload.", 40000))
	var sawRange bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			sawRange = true
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	got, err := downloadViaStream(t, srv.URL)
	if err != nil {
		t.Fatalf("download error: %v", err)
	}
	if !sawRange {
		t.Error("ranged probe was never attempted")
	}
	if string(got) != string(payload) {
		t.Fatalf("fallback mismatch: got %d bytes want %d", len(got), len(payload))
	}
}

// TestStreamDownloadSmallPayload covers the whole-file-fits-in-one-chunk branch
// of rangedDownload.
func TestStreamDownloadSmallPayload(t *testing.T) {
	payload := []byte("small-media-payload")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveRange(t, w, r, payload)
	}))
	defer srv.Close()

	got, err := downloadViaStream(t, srv.URL)
	if err != nil {
		t.Fatalf("download error: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got %q want %q", got, payload)
	}
}

// serveRange writes the byte slice honouring a "bytes=start-end" Range header.
func serveRange(t *testing.T, w http.ResponseWriter, r *http.Request, body []byte) {
	t.Helper()
	spec := r.Header.Get("Range")
	if spec == "" {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	var start, end int
	if _, err := fmt.Sscanf(spec, "bytes=%d-%d", &start, &end); err != nil {
		t.Errorf("bad range %q: %v", spec, err)
		http.Error(w, "bad range", http.StatusBadRequest)
		return
	}
	if start < 0 || start >= len(body) {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if end >= len(body) {
		end = len(body) - 1
	}
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
	w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write(body[start : end+1])
}

// downloadViaStream runs the production streamDownloadToFile against url and
// returns the bytes it wrote.
func downloadViaStream(t *testing.T, url string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	path, err := streamDownloadToFile(ctx, &http.Client{Timeout: 30 * time.Second}, url, nil)
	if err != nil {
		return nil, err
	}
	defer os.Remove(path)
	return os.ReadFile(path)
}
