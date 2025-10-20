package communities

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUpload_Success(t *testing.T) {
	// Arrange a fake Codex server that validates headers and returns a CID
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/codex/v1/data" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/octet-stream" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if cd := r.Header.Get("Content-Disposition"); cd != "filename=\"hello.txt\"" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		_, _ = io.ReadAll(r.Body) // consume body
		_ = r.Body.Close()

		w.WriteHeader(http.StatusOK)
		// Codex returns CIDv1 base58btc
		// prefix: zDv
		//   - z = multibase prefix for base58btc
		//	 - Dv = CIDv1 prefix for raw codex
		// we add a newline to simulate real response
		_, _ = w.Write([]byte("zDvZRwzmTestCID123\n"))
	}))
	defer server.Close()

	client := NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	// Act
	cid, err := client.Upload(bytes.NewReader([]byte("payload")), "hello.txt")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Codex uses CIDv1 with base58btc encoding (prefix: zDv)
	if cid != "zDvZRwzmTestCID123" {
		t.Fatalf("unexpected cid: %q", cid)
	}
}

func TestDownload_Success(t *testing.T) {
	const wantCID = "zDvZRwzm"
	const payload = "hello from codex"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/codex/v1/data/"+wantCID+"/network/stream" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	client := NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	var buf bytes.Buffer
	if err := client.Download(wantCID, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := buf.String(); got != payload {
		t.Fatalf("unexpected payload: %q", got)
	}
}

func TestDownloadWithContext_Cancel(t *testing.T) {
	const cid = "zDvZRwzm"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/codex/v1/data/"+cid+"/network/stream" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		flusher, _ := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		// Stream data slowly so the request can be canceled
		for i := 0; i < 1000; i++ {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			if _, err := w.Write([]byte("x")); err != nil {
				// Client likely went away; stop writing
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer server.Close()

	client := NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := client.DownloadWithContext(ctx, cid, io.Discard)
	if err == nil {
		t.Fatalf("expected cancellation error, got nil")
	}
	// Accept either canceled or deadline exceeded depending on timing
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		// net/http may wrap the context error; check error string as a fallback
		es := err.Error()
		if !(es == context.Canceled.Error() || es == context.DeadlineExceeded.Error()) {
			t.Fatalf("expected context cancellation, got: %v", err)
		}
	}
}
