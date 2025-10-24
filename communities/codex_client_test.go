package communities_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-codex-client/communities"
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

	client := communities.NewCodexClient("localhost", "8080")
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

	client := communities.NewCodexClient("localhost", "8080")
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

	client := communities.NewCodexClient("localhost", "8080")
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

func TestHasCid_Success(t *testing.T) {
	tests := []struct {
		name     string
		cid      string
		hasIt    bool
		wantBool bool
	}{
		{"has CID returns true", "zDvZRwzmTestCID", true, true},
		{"has CID returns false", "zDvZRwzmTestCID", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/codex/v1/data/"+tt.cid+"/exists" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				// Return JSON: {"<cid>": <bool>}
				fmt.Fprintf(w, `{"%s": %t}`, tt.cid, tt.hasIt)
			}))
			defer server.Close()

			client := communities.NewCodexClient("localhost", "8080")
			client.BaseURL = server.URL

			got, err := client.HasCid(tt.cid)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantBool {
				t.Fatalf("HasCid(%q) = %v, want %v", tt.cid, got, tt.wantBool)
			}
		})
	}
}

func TestHasCid_RequestError(t *testing.T) {
	// Create a server and immediately close it to trigger connection error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close() // Close immediately so connection fails

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL // Use the closed server's URL

	got, err := client.HasCid("zDvZRwzmTestCID")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != false {
		t.Fatalf("expected false on error, got %v", got)
	}
}

func TestHasCid_CidMismatch(t *testing.T) {
	const requestCid = "zDvZRwzmRequestCID"
	const responseCid = "zDvZRwzmDifferentCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Return a different CID in the response
		fmt.Fprintf(w, `{"%s": true}`, responseCid)
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	got, err := client.HasCid(requestCid)
	if err == nil {
		t.Fatal("expected error for CID mismatch, got nil")
	}
	if got != false {
		t.Fatalf("expected false on CID mismatch, got %v", got)
	}
	// Check error message mentions the missing/mismatched CID
	if !strings.Contains(err.Error(), requestCid) {
		t.Fatalf("error should mention request CID %q, got: %v", requestCid, err)
	}
}

func TestRemoveCid_Success(t *testing.T) {
	const testCid = "zDvZRwzmTestCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/codex/v1/data/"+testCid {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// DELETE should return 204 No Content
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	err := client.RemoveCid(testCid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveCid_Error(t *testing.T) {
	const testCid = "zDvZRwzmTestCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return error status
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	err := client.RemoveCid(testCid)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error should mention status 500, got: %v", err)
	}
}

func TestTriggerDownload(t *testing.T) {
	const testCid = "zDvZRwzmTestCID"
	const expectedManifest = `{
		"cid": "zDvZRwzmTestCID",
		"manifest": {
			"treeCid": "zDvZRwzmTreeCID",
			"datasetSize": 1024,
			"blockSize": 65536,
			"protected": false,
			"filename": "test-file.bin",
			"mimetype": "application/octet-stream"
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/codex/v1/data/"+testCid+"/network" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedManifest))
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	ctx := context.Background()
	manifest, err := client.TriggerDownloadWithContext(ctx, testCid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manifest.CID != testCid {
		t.Fatalf("expected CID %q, got %q", testCid, manifest.CID)
	}
	if manifest.Manifest.TreeCid != "zDvZRwzmTreeCID" {
		t.Fatalf("expected TreeCid %q, got %q", "zDvZRwzmTreeCID", manifest.Manifest.TreeCid)
	}
	if manifest.Manifest.DatasetSize != 1024 {
		t.Fatalf("expected DatasetSize %d, got %d", 1024, manifest.Manifest.DatasetSize)
	}
	if manifest.Manifest.Filename != "test-file.bin" {
		t.Fatalf("expected Filename %q, got %q", "test-file.bin", manifest.Manifest.Filename)
	}
}

func TestTriggerDownloadWithContext_RequestError(t *testing.T) {
	// Create a server and immediately close it to trigger connection error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	ctx := context.Background()
	manifest, err := client.TriggerDownloadWithContext(ctx, "zDvZRwzmRigWseNB7WqmudkKAPgZmrDCE9u5cY4KvCqhRo9Ki")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if manifest != nil {
		t.Fatalf("expected nil manifest on error, got %v", manifest)
	}
}

func TestTriggerDownloadWithContext_JSONParseError(t *testing.T) {
	const testCid = "zDvZRwzmTestCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Return invalid JSON
		w.Write([]byte(`{"invalid": json}`))
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	ctx := context.Background()
	manifest, err := client.TriggerDownloadWithContext(ctx, testCid)
	if err == nil {
		t.Fatal("expected JSON parse error, got nil")
	}
	if manifest != nil {
		t.Fatalf("expected nil manifest on parse error, got %v", manifest)
	}
	if !strings.Contains(err.Error(), "failed to parse download manifest") {
		t.Fatalf("error should mention parse failure, got: %v", err)
	}
}

func TestTriggerDownloadWithContext_HTTPError(t *testing.T) {
	const testCid = "zDvZRwzmTestCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("CID not found"))
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	ctx := context.Background()
	manifest, err := client.TriggerDownloadWithContext(ctx, testCid)
	if err == nil {
		t.Fatal("expected error for 404 status, got nil")
	}
	if manifest != nil {
		t.Fatalf("expected nil manifest on HTTP error, got %v", manifest)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error should mention status 404, got: %v", err)
	}
}

func TestTriggerDownloadWithContext_Cancellation(t *testing.T) {
	const testCid = "zDvZRwzmTestCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response to allow cancellation
		select {
		case <-r.Context().Done():
			return
		case <-time.After(200 * time.Millisecond):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"cid": "test"}`))
		}
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	// Cancel after 50ms (before server responds)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	manifest, err := client.TriggerDownloadWithContext(ctx, testCid)
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if manifest != nil {
		t.Fatalf("expected nil manifest on cancellation, got %v", manifest)
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

func TestLocalDownload(t *testing.T) {
	testData := []byte("test data for local download")
	testCid := "zDvZRwzmTestCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		expectedPath := "/api/codex/v1/data/" + testCid
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		w.Write(testData)
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	var buf bytes.Buffer
	err := client.LocalDownload(testCid, &buf)
	if err != nil {
		t.Fatalf("LocalDownload failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), testData) {
		t.Errorf("Downloaded data mismatch. Expected %q, got %q", string(testData), buf.String())
	}
}

func TestLocalDownloadWithContext_Success(t *testing.T) {
	testData := []byte("test data for local download with context")
	testCid := "zDvZRwzmTestCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		expectedPath := "/api/codex/v1/data/" + testCid
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		w.Write(testData)
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	ctx := context.Background()
	var buf bytes.Buffer
	err := client.LocalDownloadWithContext(ctx, testCid, &buf)
	if err != nil {
		t.Fatalf("LocalDownloadWithContext failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), testData) {
		t.Errorf("Downloaded data mismatch. Expected %q, got %q", string(testData), buf.String())
	}
}

func TestLocalDownloadWithContext_RequestError(t *testing.T) {
	// Create a server and immediately close it to trigger connection error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	ctx := context.Background()
	var buf bytes.Buffer
	err := client.LocalDownloadWithContext(ctx, "zDvZRwzmTestCID", &buf)
	if err == nil {
		t.Fatal("Expected error due to closed server, got nil")
	}

	if !strings.Contains(err.Error(), "failed to download from codex") {
		t.Errorf("Expected 'failed to download from codex' in error, got: %v", err)
	}
}

func TestLocalDownloadWithContext_HTTPError(t *testing.T) {
	testCid := "zDvZRwzmTestCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("CID not found in local storage"))
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	ctx := context.Background()
	var buf bytes.Buffer
	err := client.LocalDownloadWithContext(ctx, testCid, &buf)
	if err == nil {
		t.Fatal("Expected error for HTTP 404, got nil")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Expected '404' in error message, got: %v", err)
	}
}

func TestLocalDownloadWithContext_Cancellation(t *testing.T) {
	testCid := "zDvZRwzmTestCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("slow response"))
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	var buf bytes.Buffer
	err := client.LocalDownloadWithContext(ctx, testCid, &buf)
	if err == nil {
		t.Fatal("Expected context cancellation error, got nil")
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

func TestFetchManifestWithContext_Success(t *testing.T) {
	testCid := "zDvZRwzmTestCID"
	expectedManifest := `{
		"cid": "zDvZRwzmTestCID",
		"manifest": {
			"treeCid": "zDvZRwzmTreeCID123",
			"datasetSize": 1024,
			"blockSize": 256,
			"protected": true,
			"filename": "test-file.bin",
			"mimetype": "application/octet-stream"
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		expectedPath := fmt.Sprintf("/api/codex/v1/data/%s/network/manifest", testCid)
		if r.URL.Path != expectedPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedManifest))
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	ctx := context.Background()
	manifest, err := client.FetchManifestWithContext(ctx, testCid)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if manifest == nil {
		t.Fatal("Expected manifest, got nil")
	}

	if manifest.CID != testCid {
		t.Errorf("Expected CID %s, got %s", testCid, manifest.CID)
	}

	if manifest.Manifest.TreeCid != "zDvZRwzmTreeCID123" {
		t.Errorf("Expected TreeCid %s, got %s", "zDvZRwzmTreeCID123", manifest.Manifest.TreeCid)
	}

	if manifest.Manifest.DatasetSize != 1024 {
		t.Errorf("Expected DatasetSize %d, got %d", 1024, manifest.Manifest.DatasetSize)
	}

	if manifest.Manifest.BlockSize != 256 {
		t.Errorf("Expected BlockSize %d, got %d", 256, manifest.Manifest.BlockSize)
	}

	if !manifest.Manifest.Protected {
		t.Error("Expected Protected to be true, got false")
	}

	if manifest.Manifest.Filename != "test-file.bin" {
		t.Errorf("Expected Filename %s, got %s", "test-file.bin", manifest.Manifest.Filename)
	}

	if manifest.Manifest.Mimetype != "application/octet-stream" {
		t.Errorf("Expected Mimetype %s, got %s", "application/octet-stream", manifest.Manifest.Mimetype)
	}
}

func TestFetchManifestWithContext_RequestError(t *testing.T) {
	client := communities.NewCodexClient("invalid-host", "8080")

	ctx := context.Background()
	manifest, err := client.FetchManifestWithContext(ctx, "test-cid")
	if err == nil {
		t.Fatal("Expected error for invalid host, got nil")
	}
	if manifest != nil {
		t.Fatal("Expected nil manifest on error, got non-nil")
	}

	if !strings.Contains(err.Error(), "failed to fetch manifest from codex") {
		t.Errorf("Expected 'failed to fetch manifest from codex' in error message, got: %v", err)
	}
}

func TestFetchManifestWithContext_HTTPError(t *testing.T) {
	testCid := "zDvZRwzmTestCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Manifest not found"))
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	ctx := context.Background()
	manifest, err := client.FetchManifestWithContext(ctx, testCid)
	if err == nil {
		t.Fatal("Expected error for HTTP 404, got nil")
	}
	if manifest != nil {
		t.Fatal("Expected nil manifest on error, got non-nil")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Expected '404' in error message, got: %v", err)
	}
}

func TestFetchManifestWithContext_JSONParseError(t *testing.T) {
	testCid := "zDvZRwzmTestCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json {"))
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	ctx := context.Background()
	manifest, err := client.FetchManifestWithContext(ctx, testCid)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
	if manifest != nil {
		t.Fatal("Expected nil manifest on JSON parse error, got non-nil")
	}

	if !strings.Contains(err.Error(), "failed to parse manifest") {
		t.Errorf("Expected 'failed to parse manifest' in error message, got: %v", err)
	}
}

func TestFetchManifestWithContext_Cancellation(t *testing.T) {
	testCid := "zDvZRwzmTestCID"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow response
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"cid": "test"}`))
	}))
	defer server.Close()

	client := communities.NewCodexClient("localhost", "8080")
	client.BaseURL = server.URL

	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	manifest, err := client.FetchManifestWithContext(ctx, testCid)
	if err == nil {
		t.Fatal("Expected context cancellation error, got nil")
	}
	if manifest != nil {
		t.Fatal("Expected nil manifest on cancellation, got non-nil")
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
