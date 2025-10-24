//go:build integration
// +build integration

package communities_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"go-codex-client/communities"
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
	client := communities.NewCodexClient(host, port)

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

	// Clean up after test
	defer func() {
		if err := client.RemoveCid(cid); err != nil {
			t.Logf("Warning: Failed to remove CID %s: %v", cid, err)
		}
	}()

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
	client := communities.NewCodexClient(host, port)

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

func TestIntegration_TriggerDownload(t *testing.T) {
	host := getenv("CODEX_HOST", "localhost")
	port := getenv("CODEX_API_PORT", "8001") // Use port 8001 as specified by user
	client := communities.NewCodexClient(host, port)

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

	// Upload the data
	cid, err := client.Upload(bytes.NewReader(payload), "local-download-test.bin")
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	t.Logf("Upload successful, CID: %s", cid)

	// Clean up after test
	defer func() {
		if err := client.RemoveCid(cid); err != nil {
			t.Logf("Warning: Failed to remove CID %s: %v", cid, err)
		}
	}()

	// Trigger async download
	manifest, err := client.TriggerDownload(cid)
	if err != nil {
		t.Fatalf("TriggerDownload failed: %v", err)
	}
	t.Logf("Async download triggered, manifest CID: %s", manifest.CID)

	// Poll HasCid for up to 10 seconds using goroutine and channel
	downloadComplete := make(chan bool, 1)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			hasCid, err := client.HasCid(cid)
			if err != nil {
				t.Logf("HasCid check failed: %v", err)
				continue
			}
			if hasCid {
				t.Logf("CID is now available locally")
				downloadComplete <- true
				return
			} else {
				t.Logf("CID not yet available locally, continuing to poll...")
			}
		}
	}()

	// Wait for download completion or timeout
	select {
	case <-downloadComplete:
		// Download completed successfully
	case <-time.After(10 * time.Second):
		t.Fatalf("Timeout waiting for CID to be available locally after 10 seconds")
	}

	// Now download the actual content from local storage and verify it matches
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var downloadBuf bytes.Buffer
	if err := client.LocalDownloadWithContext(ctx, cid, &downloadBuf); err != nil {
		t.Fatalf("LocalDownload after trigger download failed: %v", err)
	}

	downloadedData := downloadBuf.Bytes()
	t.Logf("Downloaded data (first 32 bytes hex): %s", hex.EncodeToString(downloadedData[:32]))

	// Verify the data matches
	if !bytes.Equal(payload, downloadedData) {
		t.Errorf("Downloaded data does not match uploaded data")
		t.Errorf("Expected length: %d, got: %d", len(payload), len(downloadedData))
	}
}

func TestIntegration_FetchManifest(t *testing.T) {
	host := getenv("CODEX_HOST", "localhost")
	port := getenv("CODEX_API_PORT", "8080")
	client := communities.NewCodexClient(host, port)

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

	cid, err := client.Upload(bytes.NewReader(payload), "fetch-manifest-test.bin")
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	t.Logf("Upload successful, CID: %s", cid)

	// Clean up after test
	defer func() {
		if err := client.RemoveCid(cid); err != nil {
			t.Logf("Warning: Failed to remove CID %s: %v", cid, err)
		}
	}()

	// Verify existence via HasCid first
	exists, err := client.HasCid(cid)
	if err != nil {
		t.Fatalf("HasCid failed: %v", err)
	}
	if !exists {
		t.Fatalf("HasCid returned false for uploaded CID %s", cid)
	}
	t.Logf("HasCid confirmed existence of CID: %s", cid)

	// Fetch manifest with context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manifest, err := client.FetchManifestWithContext(ctx, cid)
	if err != nil {
		t.Fatalf("FetchManifestWithContext failed: %v", err)
	}
	t.Logf("FetchManifest successful, manifest CID: %s", manifest.CID)

	// Verify manifest properties
	if manifest.CID != cid {
		t.Errorf("Manifest CID mismatch: expected %s, got %s", cid, manifest.CID)
	}

	// Verify manifest has expected fields
	if manifest.Manifest.TreeCid == "" {
		t.Error("Expected TreeCid to be non-empty")
	}
	t.Logf("Manifest TreeCid: %s", manifest.Manifest.TreeCid)

	if manifest.Manifest.DatasetSize <= 0 {
		t.Errorf("Expected DatasetSize > 0, got %d", manifest.Manifest.DatasetSize)
	}
	t.Logf("Manifest DatasetSize: %d", manifest.Manifest.DatasetSize)

	if manifest.Manifest.BlockSize <= 0 {
		t.Errorf("Expected BlockSize > 0, got %d", manifest.Manifest.BlockSize)
	}
	t.Logf("Manifest BlockSize: %d", manifest.Manifest.BlockSize)

	if manifest.Manifest.Filename != "fetch-manifest-test.bin" {
		t.Errorf("Expected Filename 'fetch-manifest-test.bin', got '%s'", manifest.Manifest.Filename)
	}
	t.Logf("Manifest Filename: %s", manifest.Manifest.Filename)

	// Log manifest details for verification
	t.Logf("Manifest Protected: %v", manifest.Manifest.Protected)
	t.Logf("Manifest Mimetype: %s", manifest.Manifest.Mimetype)

	// Test fetching manifest for non-existent CID (should fail gracefully)
	nonExistentCID := "zDvZRwzmNonExistentCID123456789"
	_, err = client.FetchManifestWithContext(ctx, nonExistentCID)
	if err == nil {
		t.Error("Expected error when fetching manifest for non-existent CID, got nil")
	} else {
		t.Logf("Expected error for non-existent CID: %v", err)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
