//go:build integration
// +build integration

package communities

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"
)

// This test exercises real network calls against a running Codex node.
// It is disabled by default via the "integration" build tag.
// Run with:
//
//	go test -v -tags=integration ./communities -run Integration
//
// Required env vars (with defaults):
//
//	CODEX_HOST (default: localhost)
//	CODEX_API_PORT (default: 8080)
//	CODEX_TIMEOUT_MS (optional; default: 60000)
func TestIntegration_UploadAndDownload(t *testing.T) {
	host := getenv("CODEX_HOST", "localhost")
	port := getenv("CODEX_API_PORT", "8080")
	client := NewCodexClient(host, port)

	// Optional request timeout override
	if ms := os.Getenv("CODEX_TIMEOUT_MS"); ms != "" {
		if d, err := time.ParseDuration(ms + "ms"); err == nil {
			client.SetRequestTimeout(d)
		}
	}

	// Generate random payload to ensure proper round-trip verification
	payload := make([]byte, 1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("failed to generate random payload: %v", err)
	}
	t.Logf("Generated payload (first 32 bytes hex): %s", hex.EncodeToString(payload[:32]))

	cid, err := client.Upload(bytes.NewReader(payload), "it.bin")
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	t.Logf("Upload successful, CID: %s", cid)

	// Verify existence via HasCid
	exists, err := client.HasCid(cid)
	if err != nil {
		t.Fatalf("HasCid failed: %v", err)
	}
	if !exists {
		t.Fatalf("HasCid returned false for uploaded CID %s", cid)
	}
	t.Logf("HasCid confirmed existence of CID: %s", cid)

	// Download via network stream with a context timeout to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var buf bytes.Buffer
	if err := client.DownloadWithContext(ctx, cid, &buf); err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if got := buf.Bytes(); !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %q want %q", string(got), string(payload))
	}
}

func TestIntegration_CheckNonExistingCID(t *testing.T) {
	host := getenv("CODEX_HOST", "localhost")
	port := getenv("CODEX_API_PORT", "8080")
	client := NewCodexClient(host, port)

	// Generate random payload to ensure proper round-trip verification
	payload := make([]byte, 1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("failed to generate random payload: %v", err)
	}
	t.Logf("Generated payload (first 32 bytes hex): %s", hex.EncodeToString(payload[:32]))

	cid, err := client.Upload(bytes.NewReader(payload), "it.bin")
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	t.Logf("Upload successful, CID: %s", cid)

	// Verify existence via HasCid
	exists, err := client.HasCid(cid)
	if err != nil {
		t.Fatalf("HasCid failed: %v", err)
	}
	if !exists {
		t.Fatalf("HasCid returned false for uploaded CID %s", cid)
	}
	t.Logf("HasCid confirmed existence of CID: %s", cid)

	// Remove CID from Codex
	if err := client.RemoveCid(cid); err != nil {
		t.Fatalf("RemoveCid failed: %v", err)
	}
	t.Logf("RemoveCid confirmed deletion of CID: %s", cid)

	exists, err = client.HasCid(cid)
	if err != nil {
		t.Fatalf("HasCid failed: %v", err)
	}
	if exists {
		t.Fatalf("HasCid returned true for removed CID %s", cid)
	}
	t.Logf("HasCid confirmed CID is no longer present: %s", cid)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
