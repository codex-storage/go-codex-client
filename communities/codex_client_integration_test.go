//go:build codex_integration
// +build codex_integration

package communities_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/codex-storage/codex-go-bindings/codex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"go-codex-client/communities"
)

// CodexClientIntegrationTestSuite demonstrates testify's suite functionality for CodexClient integration tests
type CodexClientIntegrationTestSuite struct {
	suite.Suite
	client *communities.CodexClient
}

// SetupSuite runs once before all tests in the suite
func (suite *CodexClientIntegrationTestSuite) SetupSuite() {
	var err error
	suite.client, err = communities.NewCodexClient(codex.Config{
		DataDir:        suite.T().TempDir(),
		LogFormat:      codex.LogFormatNoColors,
		MetricsEnabled: false,
		BlockRetries:   5,
	})
	if err != nil {
		suite.T().Fatalf("Failed to create Codex client: %v", err)
	}
}

// TestCodexClientIntegrationTestSuite runs the integration test suite
func TestCodexClientIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CodexClientIntegrationTestSuite))
}

func (suite *CodexClientIntegrationTestSuite) TestIntegration_UploadAndDownload() {
	// Generate random payload to ensure proper round-trip verification
	payload := make([]byte, 1024)
	_, err := rand.Read(payload)
	require.NoError(suite.T(), err, "failed to generate random payload")
	suite.T().Logf("Generated payload (first 32 bytes hex): %s", hex.EncodeToString(payload[:32]))

	cid, err := suite.client.Upload(bytes.NewReader(payload), "it.bin")
	require.NoError(suite.T(), err, "upload failed")
	suite.T().Logf("Upload successful, CID: %s", cid)

	// Clean up after test
	defer func() {
		if err := suite.client.RemoveCid(cid); err != nil {
			suite.T().Logf("Warning: Failed to remove CID %s: %v", cid, err)
		}
	}()

	// Verify existence via HasCid
	exists, err := suite.client.HasCid(cid)
	require.NoError(suite.T(), err, "HasCid failed")
	assert.True(suite.T(), exists, "HasCid returned false for uploaded CID %s", cid)
	suite.T().Logf("HasCid confirmed existence of CID: %s", cid)

	// Download via network stream with a context timeout to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var buf bytes.Buffer
	err = suite.client.DownloadWithContext(ctx, cid, &buf)
	require.NoError(suite.T(), err, "download failed")
	assert.Equal(suite.T(), payload, buf.Bytes(), "payload mismatch")
}

func (suite *CodexClientIntegrationTestSuite) TestIntegration_CheckNonExistingCID() {
	// Generate random payload to ensure proper round-trip verification
	payload := make([]byte, 1024)
	_, err := rand.Read(payload)
	require.NoError(suite.T(), err, "failed to generate random payload")
	suite.T().Logf("Generated payload (first 32 bytes hex): %s", hex.EncodeToString(payload[:32]))

	cid, err := suite.client.Upload(bytes.NewReader(payload), "it.bin")
	require.NoError(suite.T(), err, "upload failed")
	suite.T().Logf("Upload successful, CID: %s", cid)

	// Verify existence via HasCid
	exists, err := suite.client.HasCid(cid)
	require.NoError(suite.T(), err, "HasCid failed")
	assert.True(suite.T(), exists, "HasCid returned false for uploaded CID %s", cid)
	suite.T().Logf("HasCid confirmed existence of CID: %s", cid)

	// Remove CID from Codex
	err = suite.client.RemoveCid(cid)
	require.NoError(suite.T(), err, "RemoveCid failed")
	suite.T().Logf("RemoveCid confirmed deletion of CID: %s", cid)

	exists, err = suite.client.HasCid(cid)
	require.NoError(suite.T(), err, "HasCid failed after removal")
	assert.False(suite.T(), exists, "HasCid returned true for removed CID %s", cid)
	suite.T().Logf("HasCid confirmed CID is no longer present: %s", cid)
}

func (suite *CodexClientIntegrationTestSuite) TestIntegration_TriggerDownload() {
	client := communities.NewCodexClientTest(suite.T())

	// Generate random payload to ensure proper round-trip verification
	payload := make([]byte, 1024)
	_, err := rand.Read(payload)
	require.NoError(suite.T(), err, "failed to generate random payload")
	suite.T().Logf("Generated payload (first 32 bytes hex): %s", hex.EncodeToString(payload[:32]))

	// Upload the data
	cid, err := client.Upload(bytes.NewReader(payload), "local-download-test.bin")
	require.NoError(suite.T(), err, "upload failed")
	suite.T().Logf("Upload successful, CID: %s", cid)

	// Clean up after test
	defer func() {
		if err := client.RemoveCid(cid); err != nil {
			suite.T().Logf("Warning: Failed to remove CID %s: %v", cid, err)
		}
	}()

	// Trigger async download
	manifest, err := client.TriggerDownload(cid)
	require.NoError(suite.T(), err, "TriggerDownload failed")
	suite.T().Logf("Async download triggered, manifest CID: %s", manifest.Cid)

	// Poll HasCid for up to 10 seconds using goroutine and channel
	downloadComplete := make(chan bool, 1)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			hasCid, err := client.HasCid(cid)
			if err != nil {
				suite.T().Logf("HasCid check failed: %v", err)
				continue
			}
			if hasCid {
				suite.T().Logf("CID is now available locally")
				downloadComplete <- true
				return
			} else {
				suite.T().Logf("CID not yet available locally, continuing to poll...")
			}
		}
	}()

	// Wait for download completion or timeout
	select {
	case <-downloadComplete:
		// Download completed successfully
	case <-time.After(10 * time.Second):
		suite.T().Fatalf("Timeout waiting for CID to be available locally after 10 seconds")
	}

	// Now download the actual content from local storage and verify it matches
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var downloadBuf bytes.Buffer
	err = client.LocalDownloadWithContext(ctx, cid, &downloadBuf)
	require.NoError(suite.T(), err, "LocalDownload after trigger download failed")

	downloadedData := downloadBuf.Bytes()
	suite.T().Logf("Downloaded data (first 32 bytes hex): %s", hex.EncodeToString(downloadedData[:32]))

	// Verify the data matches
	assert.Equal(suite.T(), payload, downloadedData, "Downloaded data does not match uploaded data")
}

func (suite *CodexClientIntegrationTestSuite) TestIntegration_FetchManifest() {
	// Generate random payload to ensure proper round-trip verification
	payload := make([]byte, 1024)
	_, err := rand.Read(payload)
	require.NoError(suite.T(), err, "failed to generate random payload")
	suite.T().Logf("Generated payload (first 32 bytes hex): %s", hex.EncodeToString(payload[:32]))

	cid, err := suite.client.Upload(bytes.NewReader(payload), "fetch-manifest-test.bin")
	require.NoError(suite.T(), err, "upload failed")
	suite.T().Logf("Upload successful, CID: %s", cid)

	// Clean up after test
	defer func() {
		if err := suite.client.RemoveCid(cid); err != nil {
			suite.T().Logf("Warning: Failed to remove CID %s: %v", cid, err)
		}
	}()

	// Verify existence via HasCid first
	exists, err := suite.client.HasCid(cid)
	require.NoError(suite.T(), err, "HasCid failed")
	assert.True(suite.T(), exists, "HasCid returned false for uploaded CID %s", cid)
	suite.T().Logf("HasCid confirmed existence of CID: %s", cid)

	// Fetch manifest with context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manifest, err := suite.client.FetchManifestWithContext(ctx, cid)
	require.NoError(suite.T(), err, "FetchManifestWithContext failed")
	suite.T().Logf("FetchManifest successful, manifest CID: %s", manifest.Cid)

	// Verify manifest properties
	assert.Equal(suite.T(), cid, manifest.Cid, "Manifest CID mismatch")

	// Verify manifest has expected fields
	assert.NotEmpty(suite.T(), manifest.TreeCid, "Expected TreeCid to be non-empty")
	suite.T().Logf("Manifest TreeCid: %s", manifest.TreeCid)

	assert.Greater(suite.T(), manifest.DatasetSize, 0, "Expected DatasetSize > 0")
	suite.T().Logf("Manifest DatasetSize: %d", manifest.DatasetSize)

	assert.Greater(suite.T(), manifest.BlockSize, 0, "Expected BlockSize > 0")
	suite.T().Logf("Manifest BlockSize: %d", manifest.BlockSize)

	assert.Equal(suite.T(), "fetch-manifest-test.bin", manifest.Filename, "Filename mismatch")
	suite.T().Logf("Manifest Filename: %s", manifest.Filename)

	// Log manifest details for verification
	suite.T().Logf("Manifest Protected: %v", manifest.Protected)
	suite.T().Logf("Manifest Mimetype: %s", manifest.Mimetype)

	// Test fetching manifest for non-existent CID (should fail gracefully)
	nonExistentCID := "zDvZRwzmNonExistentCID123456789"
	_, err = suite.client.FetchManifestWithContext(ctx, nonExistentCID)
	assert.Error(suite.T(), err, "Expected error when fetching manifest for non-existent CID")
	suite.T().Logf("Expected error for non-existent CID: %v", err)
}
