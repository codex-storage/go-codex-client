//go:build !disable_torrent
// +build !disable_torrent

package communities_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"go-codex-client/communities"
	mock_communities "go-codex-client/communities/mock"
	"go-codex-client/protobuf"

	"go.uber.org/mock/gomock"
)

// CodexArchiveDownloaderTestifySuite demonstrates testify's suite functionality
type CodexArchiveDownloaderTestifySuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockClient *mock_communities.MockCodexClientInterface
	index      *protobuf.CodexWakuMessageArchiveIndex
}

// SetupTest runs before each test method
func (suite *CodexArchiveDownloaderTestifySuite) SetupTest() {
	suite.ctrl = gomock.NewController(suite.T())
	suite.mockClient = mock_communities.NewMockCodexClientInterface(suite.ctrl)
	suite.index = &protobuf.CodexWakuMessageArchiveIndex{
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

// TearDownTest runs after each test method
func (suite *CodexArchiveDownloaderTestifySuite) TearDownTest() {
	suite.ctrl.Finish()
}

func (suite *CodexArchiveDownloaderTestifySuite) TestBasicSingleArchive() {
	// Test data
	communityID := "test-community"
	existingArchiveIDs := []string{} // No existing archives
	cancelChan := make(chan struct{})

	// Set up mock expectations - same as before
	suite.mockClient.EXPECT().
		TriggerDownloadWithContext(gomock.Any(), "test-cid-1").
		Return(&communities.CodexManifest{CID: "test-cid-1"}, nil).
		Times(1)

	// First HasCid call returns false, second returns true (simulating polling)
	gomock.InOrder(
		suite.mockClient.EXPECT().HasCid("test-cid-1").Return(false, nil),
		suite.mockClient.EXPECT().HasCid("test-cid-1").Return(true, nil),
	)

	// Create downloader with mock client
	downloader := communities.NewCodexArchiveDownloader(suite.mockClient, suite.index, communityID, existingArchiveIDs, cancelChan)

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

	// Verify initial state - compare testify vs standard assertions
	// Testify version is more readable and provides better error messages
	assert.Equal(suite.T(), 1, downloader.GetTotalArchivesCount(), "Total archives count should be 1")
	assert.Equal(suite.T(), 0, downloader.GetTotalDownloadedArchivesCount(), "Downloaded archives should be 0 initially")
	assert.False(suite.T(), downloader.IsDownloadComplete(), "Download should not be complete initially")

	// Start the download
	downloader.StartDownload()

	// Wait for download to complete (with timeout)
	require.Eventually(suite.T(), func() bool {
		return downloader.IsDownloadComplete()
	}, 5*time.Second, 100*time.Millisecond, "Download should complete within 5 seconds")

	// Verify final state - testify makes these assertions more expressive
	assert.True(suite.T(), downloader.IsDownloadComplete(), "Download should be complete")
	assert.Equal(suite.T(), 1, downloader.GetTotalDownloadedArchivesCount(), "Should have 1 downloaded archive")

	// Verify callback was invoked - multiple related assertions grouped logically
	assert.True(suite.T(), callbackInvoked, "Callback should be invoked")
	assert.Equal(suite.T(), "test-archive-hash-1", callbackHash, "Callback hash should match expected")
	assert.Equal(suite.T(), uint64(1000), callbackFrom, "Callback from should be 1000")
	assert.Equal(suite.T(), uint64(2000), callbackTo, "Callback to should be 2000")

	suite.T().Log("✅ Basic single archive download test passed")
	suite.T().Log("   - All mock expectations satisfied")
	suite.T().Logf("   - Callback invoked: %v", callbackInvoked)
}

func (suite *CodexArchiveDownloaderTestifySuite) TestMultipleArchives() {
	// Create test data with multiple archives
	index := &protobuf.CodexWakuMessageArchiveIndex{
		Archives: map[string]*protobuf.CodexWakuMessageArchiveIndexMetadata{
			"archive-1": {
				Cid:      "cid-1",
				Metadata: &protobuf.WakuMessageArchiveMetadata{From: 1000, To: 2000},
			},
			"archive-2": {
				Cid:      "cid-2",
				Metadata: &protobuf.WakuMessageArchiveMetadata{From: 2000, To: 3000},
			},
			"archive-3": {
				Cid:      "cid-3",
				Metadata: &protobuf.WakuMessageArchiveMetadata{From: 3000, To: 4000},
			},
		},
	}

	communityID := "test-community"
	existingArchiveIDs := []string{} // No existing archives
	cancelChan := make(chan struct{})

	// Set up expectations for all 3 archives - testify makes verification cleaner
	expectedCids := []string{"cid-1", "cid-2", "cid-3"}

	for _, cid := range expectedCids {
		suite.mockClient.EXPECT().
			TriggerDownloadWithContext(gomock.Any(), cid).
			Return(&communities.CodexManifest{CID: cid}, nil).
			Times(1)

		// Each archive becomes available after one poll
		gomock.InOrder(
			suite.mockClient.EXPECT().HasCid(cid).Return(false, nil),
			suite.mockClient.EXPECT().HasCid(cid).Return(true, nil),
		)
	}

	// Create downloader
	downloader := communities.NewCodexArchiveDownloader(suite.mockClient, index, communityID, existingArchiveIDs, cancelChan)
	downloader.SetPollingInterval(10 * time.Millisecond)

	// Track completed archives
	completedArchives := make(map[string]bool)
	downloader.SetOnArchiveDownloaded(func(hash string, from, to uint64) {
		completedArchives[hash] = true
	})

	// Initial state verification
	assert.Equal(suite.T(), 3, downloader.GetTotalArchivesCount(), "Should have 3 total archives")
	assert.Equal(suite.T(), 0, downloader.GetTotalDownloadedArchivesCount(), "Should start with 0 downloaded")
	assert.False(suite.T(), downloader.IsDownloadComplete(), "Should not be complete initially")

	// Start download
	downloader.StartDownload()

	// Wait for all downloads to complete
	require.Eventually(suite.T(), func() bool {
		return downloader.IsDownloadComplete()
	}, 10*time.Second, 100*time.Millisecond, "All downloads should complete within 10 seconds")

	// Final state verification - testify makes these checks very readable
	assert.True(suite.T(), downloader.IsDownloadComplete(), "Download should be complete")
	assert.Equal(suite.T(), 3, downloader.GetTotalDownloadedArchivesCount(), "Should have downloaded all 3 archives")

	// Verify all archives were processed
	assert.Len(suite.T(), completedArchives, 3, "Should have completed exactly 3 archives")
	assert.Contains(suite.T(), completedArchives, "archive-1", "Should have completed archive-1")
	assert.Contains(suite.T(), completedArchives, "archive-2", "Should have completed archive-2")
	assert.Contains(suite.T(), completedArchives, "archive-3", "Should have completed archive-3")

	suite.T().Log("✅ Multiple archives test passed")
	suite.T().Logf("   - Completed %d out of %d archives", len(completedArchives), 3)
}

// Run the test suite
func TestCodexArchiveDownloaderSuite(t *testing.T) {
	suite.Run(t, new(CodexArchiveDownloaderTestifySuite))
}
