//go:build !disable_torrent
// +build !disable_torrent

package communities_test

import (
	"testing"
	"time"

	"go-codex-client/communities"
	"go-codex-client/protobuf"

	mock_communities "go-codex-client/communities/mock"

	"go.uber.org/mock/gomock"
)

// Helper function to create a test index with a single archive
func createTestIndex() *protobuf.CodexWakuMessageArchiveIndex {
	return &protobuf.CodexWakuMessageArchiveIndex{
		Archives: map[string]*protobuf.CodexWakuMessageArchiveIndexMetadata{
			"test-archive-hash-1": {
				Cid: "test-cid-1",
				Metadata: &protobuf.WakuMessageArchiveMetadata{
					From: 1000,
					To:   2000,
				},
			},
		},
	}
}

func TestCodexArchiveDownloader_BasicSingleArchive(t *testing.T) {
	// Create gomock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create test data
	index := createTestIndex()
	communityID := "test-community"
	existingArchiveIDs := []string{} // No existing archives
	cancelChan := make(chan struct{})

	// Create mock client using gomock
	mockClient := mock_communities.NewMockCodexClientInterface(ctrl)

	// Set up expectations
	mockClient.EXPECT().
		TriggerDownloadWithContext(gomock.Any(), "test-cid-1").
		Return(&communities.CodexManifest{CID: "test-cid-1"}, nil).
		Times(1)

	// First HasCid call returns false, second returns true (simulating polling)
	gomock.InOrder(
		mockClient.EXPECT().HasCid("test-cid-1").Return(false, nil),
		mockClient.EXPECT().HasCid("test-cid-1").Return(true, nil),
	)

	// Create downloader with mock client
	downloader := communities.NewCodexArchiveDownloader(mockClient, index, communityID, existingArchiveIDs, cancelChan)
	
	// Set fast polling interval for tests (10ms instead of default 1s)
	downloader.SetPollingInterval(10 * time.Millisecond)

	// Set up callback to track completion
	var callbackInvoked bool
	var callbackHash string
	var callbackFrom, callbackTo uint64

	downloader.SetOnArchiveDownloaded(func(hash string, from, to uint64) {
		callbackInvoked = true
		callbackHash = hash
		callbackFrom = from
		callbackTo = to
	})

	// Verify initial state
	if downloader.GetTotalArchivesCount() != 1 {
		t.Errorf("Expected 1 total archive, got %d", downloader.GetTotalArchivesCount())
	}

	if downloader.GetTotalDownloadedArchivesCount() != 0 {
		t.Errorf("Expected 0 downloaded archives initially, got %d", downloader.GetTotalDownloadedArchivesCount())
	}

	if downloader.IsDownloadComplete() {
		t.Error("Expected download to not be complete initially")
	}

	// Start the download
	downloader.StartDownload()

	// Wait for download to complete (with timeout)
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

waitLoop:
	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for download to complete")
		case <-ticker.C:
			if downloader.IsDownloadComplete() {
				break waitLoop
			}
		}
	}
	// Verify final state
	if !downloader.IsDownloadComplete() {
		t.Error("Expected download to be complete")
	}

	if downloader.GetTotalDownloadedArchivesCount() != 1 {
		t.Errorf("Expected 1 downloaded archive, got %d", downloader.GetTotalDownloadedArchivesCount())
	}

	// Verify callback was invoked
	if !callbackInvoked {
		t.Error("Expected callback to be invoked")
	}

	if callbackHash != "test-archive-hash-1" {
		t.Errorf("Expected callback hash 'test-archive-hash-1', got '%s'", callbackHash)
	}

	if callbackFrom != 1000 {
		t.Errorf("Expected callback from 1000, got %d", callbackFrom)
	}

	if callbackTo != 2000 {
		t.Errorf("Expected callback to 2000, got %d", callbackTo)
	}

	t.Logf("✅ Basic single archive download test passed")
	t.Logf("   - All mock expectations satisfied")
	t.Logf("   - Callback invoked: %v", callbackInvoked)
}
