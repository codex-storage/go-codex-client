//go:build !disable_torrent
// +build !disable_torrent

package communities_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"go-codex-client/communities"
	mock_communities "go-codex-client/communities/mock"
)

// CodexIndexDownloaderTestSuite demonstrates testify's suite functionality for CodexIndexDownloader tests
type CodexIndexDownloaderTestSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockClient *mock_communities.MockCodexClientInterface
	testDir    string
	cancelChan chan struct{}
	logger     *zap.Logger
}

// SetupTest runs before each test method
func (suite *CodexIndexDownloaderTestSuite) SetupTest() {
	suite.ctrl = gomock.NewController(suite.T())
	suite.mockClient = mock_communities.NewMockCodexClientInterface(suite.ctrl)

	// Create a temporary directory for test files
	var err error
	suite.testDir, err = os.MkdirTemp("", "codex-index-test-*")
	require.NoError(suite.T(), err)

	// Create a fresh cancel channel for each test
	suite.cancelChan = make(chan struct{})

	// Create logger
	suite.logger, _ = zap.NewDevelopment()
}

// TearDownTest runs after each test method
func (suite *CodexIndexDownloaderTestSuite) TearDownTest() {
	suite.ctrl.Finish()

	// Clean up cancel channel - check if it's still open before closing
	if suite.cancelChan != nil {
		select {
		case <-suite.cancelChan:
			// Already closed, do nothing
		default:
			// Still open, close it
			close(suite.cancelChan)
		}
	}

	// Clean up test directory
	if suite.testDir != "" {
		os.RemoveAll(suite.testDir)
	}
}

// TestCodexIndexDownloaderTestSuite runs the test suite
func TestCodexIndexDownloaderTestSuite(t *testing.T) {
	suite.Run(t, new(CodexIndexDownloaderTestSuite))
}

// ==================== GotManifest Tests ====================

func (suite *CodexIndexDownloaderTestSuite) TestGotManifest_SuccessClosesChannel() {
	testCid := "zDvZRwzmTestCID123"
	filePath := filepath.Join(suite.testDir, "index.bin")

	// Setup mock to return a successful manifest
	expectedManifest := &communities.CodexManifest{
		CID: testCid,
	}
	expectedManifest.Manifest.DatasetSize = 1024
	expectedManifest.Manifest.TreeCid = "zDvZRwzmTreeCID"
	expectedManifest.Manifest.BlockSize = 65536

	suite.mockClient.EXPECT().
		FetchManifestWithContext(gomock.Any(), testCid).
		Return(expectedManifest, nil)

	// Create downloader
	downloader := communities.NewCodexIndexDownloader(suite.mockClient, testCid, filePath, suite.cancelChan, suite.logger)

	// Call GotManifest
	manifestChan := downloader.GotManifest()

	// Wait for channel to close (with timeout)
	select {
	case <-manifestChan:
		// Success - channel closed as expected
		suite.T().Log("✅ GotManifest channel closed successfully")
	case <-time.After(1 * time.Second):
		suite.T().Fatal("Timeout waiting for GotManifest channel to close")
	}

	// Verify dataset size was recorded
	assert.Equal(suite.T(), int64(1024), downloader.GetDatasetSize(), "Dataset size should be recorded")
}

func (suite *CodexIndexDownloaderTestSuite) TestGotManifest_ErrorDoesNotCloseChannel() {
	testCid := "zDvZRwzmTestCID123"
	filePath := filepath.Join(suite.testDir, "index.bin")

	// Setup mock to return an error
	suite.mockClient.EXPECT().
		FetchManifestWithContext(gomock.Any(), testCid).
		Return(nil, errors.New("fetch error"))

	// Create downloader
	downloader := communities.NewCodexIndexDownloader(suite.mockClient, testCid, filePath, suite.cancelChan, suite.logger)

	// Call GotManifest
	manifestChan := downloader.GotManifest()

	// Channel should NOT close on error
	select {
	case <-manifestChan:
		suite.T().Fatal("GotManifest channel should NOT close on error")
	case <-time.After(200 * time.Millisecond):
		// Expected - channel did not close
		suite.T().Log("✅ GotManifest channel did not close on error (as expected)")
	}

	// Verify dataset size was NOT recorded (should be 0)
	assert.Equal(suite.T(), int64(0), downloader.GetDatasetSize(), "Dataset size should be 0 on error")
}

func (suite *CodexIndexDownloaderTestSuite) TestGotManifest_CidMismatchDoesNotCloseChannel() {
	testCid := "zDvZRwzmTestCID123"
	differentCid := "zDvZRwzmDifferentCID456"
	filePath := filepath.Join(suite.testDir, "index.bin")

	// Setup mock to return a manifest with different CID
	mismatchedManifest := &communities.CodexManifest{
		CID: differentCid, // Different CID!
	}
	mismatchedManifest.Manifest.DatasetSize = 1024

	suite.mockClient.EXPECT().
		FetchManifestWithContext(gomock.Any(), testCid).
		Return(mismatchedManifest, nil)

	// Create downloader
	downloader := communities.NewCodexIndexDownloader(suite.mockClient, testCid, filePath, suite.cancelChan, suite.logger)

	// Call GotManifest
	manifestChan := downloader.GotManifest()

	// Channel should NOT close on CID mismatch
	select {
	case <-manifestChan:
		suite.T().Fatal("GotManifest channel should NOT close on CID mismatch")
	case <-time.After(200 * time.Millisecond):
		// Expected - channel did not close
		suite.T().Log("✅ GotManifest channel did not close on CID mismatch (as expected)")
	}

	// Verify dataset size was NOT recorded (should be 0)
	assert.Equal(suite.T(), int64(0), downloader.GetDatasetSize(), "Dataset size should be 0 on CID mismatch")
}

func (suite *CodexIndexDownloaderTestSuite) TestGotManifest_Cancellation() {
	testCid := "zDvZRwzmTestCID123"
	filePath := filepath.Join(suite.testDir, "index.bin")

	// Setup mock with DoAndReturn to simulate slow response and check for cancellation
	fetchCalled := make(chan struct{})
	suite.mockClient.EXPECT().
		FetchManifestWithContext(gomock.Any(), testCid).
		DoAndReturn(func(ctx context.Context, cid string) (*communities.CodexManifest, error) {
			close(fetchCalled) // Signal that fetch was called

			// Wait for context cancellation
			<-ctx.Done()
			return nil, ctx.Err()
		})

	// Create downloader
	downloader := communities.NewCodexIndexDownloader(suite.mockClient, testCid, filePath, suite.cancelChan, suite.logger)

	// Call GotManifest
	manifestChan := downloader.GotManifest()

	// Wait for FetchManifestWithContext to be called
	select {
	case <-fetchCalled:
		suite.T().Log("FetchManifestWithContext was called")
	case <-time.After(1 * time.Second):
		suite.T().Fatal("Timeout waiting for FetchManifestWithContext to be called")
	}

	// Now trigger cancellation
	close(suite.cancelChan)

	// Channel should NOT close on cancellation
	select {
	case <-manifestChan:
		suite.T().Fatal("GotManifest channel should NOT close on cancellation")
	case <-time.After(200 * time.Millisecond):
		// Expected - channel did not close
		suite.T().Log("✅ GotManifest was cancelled and channel did not close (as expected)")
	}

	// Verify dataset size was NOT recorded
	assert.Equal(suite.T(), int64(0), downloader.GetDatasetSize(), "Dataset size should be 0 on cancellation")
}

func (suite *CodexIndexDownloaderTestSuite) TestGotManifest_RecordsDatasetSize() {
	testCid := "zDvZRwzmTestCID123"
	filePath := filepath.Join(suite.testDir, "index.bin")
	expectedSize := int64(2048)

	// Setup mock to return a manifest with specific dataset size
	expectedManifest := &communities.CodexManifest{
		CID: testCid,
	}
	expectedManifest.Manifest.DatasetSize = expectedSize
	expectedManifest.Manifest.TreeCid = "zDvZRwzmTreeCID"

	suite.mockClient.EXPECT().
		FetchManifestWithContext(gomock.Any(), testCid).
		Return(expectedManifest, nil)

	// Create downloader
	downloader := communities.NewCodexIndexDownloader(suite.mockClient, testCid, filePath, suite.cancelChan, suite.logger)

	// Initially, dataset size should be 0
	assert.Equal(suite.T(), int64(0), downloader.GetDatasetSize(), "Initial dataset size should be 0")

	// Call GotManifest
	manifestChan := downloader.GotManifest()

	// Wait for channel to close
	select {
	case <-manifestChan:
		suite.T().Log("GotManifest completed successfully")
	case <-time.After(1 * time.Second):
		suite.T().Fatal("Timeout waiting for GotManifest to complete")
	}

	// Verify dataset size was recorded correctly
	assert.Equal(suite.T(), expectedSize, downloader.GetDatasetSize(), "Dataset size should match manifest")
	suite.T().Logf("✅ Dataset size correctly recorded: %d", downloader.GetDatasetSize())
}

// ==================== DownloadIndexFile Tests ====================

func (suite *CodexIndexDownloaderTestSuite) TestDownloadIndexFile_StoresFileCorrectly() {
	testCid := "zDvZRwzmTestCID123"
	filePath := filepath.Join(suite.testDir, "downloaded-index.bin")
	testData := []byte("test index file content with some data")

	// Setup mock to write test data to the provided writer
	suite.mockClient.EXPECT().
		DownloadWithContext(gomock.Any(), testCid, gomock.Any()).
		DoAndReturn(func(ctx context.Context, cid string, w io.Writer) error {
			_, err := w.Write(testData)
			return err
		})

	// Create downloader
	downloader := communities.NewCodexIndexDownloader(suite.mockClient, testCid, filePath, suite.cancelChan, suite.logger)

	// Start download
	downloader.DownloadIndexFile()

	// Wait for download to complete (check file existence and size)
	require.Eventually(suite.T(), func() bool {
		stat, err := os.Stat(filePath)
		if err != nil {
			return false
		}
		return stat.Size() == int64(len(testData))
	}, 2*time.Second, 50*time.Millisecond, "File should be created with correct size")

	// Verify file contents
	actualData, err := os.ReadFile(filePath)
	require.NoError(suite.T(), err, "Should be able to read downloaded file")
	assert.Equal(suite.T(), testData, actualData, "File contents should match")
	suite.T().Logf("✅ File downloaded successfully to: %s", filePath)
}

func (suite *CodexIndexDownloaderTestSuite) TestDownloadIndexFile_TracksProgress() {
	testCid := "zDvZRwzmTestCID123"
	filePath := filepath.Join(suite.testDir, "progress-test.bin")
	testData := []byte("0123456789") // 10 bytes

	// Setup mock to write test data in chunks
	suite.mockClient.EXPECT().
		DownloadWithContext(gomock.Any(), testCid, gomock.Any()).
		DoAndReturn(func(ctx context.Context, cid string, w io.Writer) error {
			// Write in 2-byte chunks to simulate streaming
			for i := 0; i < len(testData); i += 2 {
				end := i + 2
				if end > len(testData) {
					end = len(testData)
				}
				_, err := w.Write(testData[i:end])
				if err != nil {
					return err
				}
				time.Sleep(10 * time.Millisecond) // Small delay to allow progress tracking
			}
			return nil
		})

	// Create downloader and set dataset size
	downloader := communities.NewCodexIndexDownloader(suite.mockClient, testCid, filePath, suite.cancelChan, suite.logger)

	// Start download
	downloader.DownloadIndexFile()

	// Initially bytes completed should be 0
	assert.Equal(suite.T(), int64(0), downloader.BytesCompleted(), "Initial bytes completed should be 0")

	// Wait for some progress
	require.Eventually(suite.T(), func() bool {
		return downloader.BytesCompleted() > 0
	}, 1*time.Second, 20*time.Millisecond, "Progress should increase")

	suite.T().Logf("Progress observed: %d bytes", downloader.BytesCompleted())

	// Wait for download to complete
	require.Eventually(suite.T(), func() bool {
		return downloader.BytesCompleted() == int64(len(testData))
	}, 2*time.Second, 50*time.Millisecond, "Should download all bytes")

	assert.Equal(suite.T(), int64(len(testData)), downloader.BytesCompleted(), "All bytes should be downloaded")
	suite.T().Logf("✅ Download progress tracked correctly: %d/%d bytes", downloader.BytesCompleted(), len(testData))
}

func (suite *CodexIndexDownloaderTestSuite) TestDownloadIndexFile_Cancellation() {
	testCid := "zDvZRwzmTestCID123"
	filePath := filepath.Join(suite.testDir, "cancel-test.bin")

	// Setup mock with DoAndReturn to simulate slow download and check for cancellation
	downloadStarted := make(chan struct{})
	suite.mockClient.EXPECT().
		DownloadWithContext(gomock.Any(), testCid, gomock.Any()).
		DoAndReturn(func(ctx context.Context, cid string, w io.Writer) error {
			close(downloadStarted) // Signal that download started

			// Simulate slow download with cancellation check
			for i := 0; i < 100; i++ {
				select {
				case <-ctx.Done():
					return ctx.Err() // Return cancellation error
				default:
					_, err := w.Write([]byte("x"))
					if err != nil {
						return err
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			return nil
		})

	// Create downloader
	downloader := communities.NewCodexIndexDownloader(suite.mockClient, testCid, filePath, suite.cancelChan, suite.logger)

	// Start download
	downloader.DownloadIndexFile()

	// Wait for download to start
	select {
	case <-downloadStarted:
		suite.T().Log("Download started")
	case <-time.After(1 * time.Second):
		suite.T().Fatal("Timeout waiting for download to start")
	}

	// Trigger cancellation
	close(suite.cancelChan)
	suite.T().Log("Cancellation triggered")

	// Wait a bit for cancellation to take effect
	time.Sleep(200 * time.Millisecond)

	// Verify that download was stopped (bytes completed should be small)
	bytesCompleted := downloader.BytesCompleted()
	suite.T().Logf("✅ Download cancelled after %d bytes (should be < 100)", bytesCompleted)
	assert.Less(suite.T(), bytesCompleted, int64(100), "Download should be cancelled before completing all 100 bytes")
}

func (suite *CodexIndexDownloaderTestSuite) TestDownloadIndexFile_ErrorHandling() {
	testCid := "zDvZRwzmTestCID123"
	filePath := filepath.Join(suite.testDir, "error-test.bin")

	// Setup mock to return an error during download
	suite.mockClient.EXPECT().
		DownloadWithContext(gomock.Any(), testCid, gomock.Any()).
		Return(errors.New("download failed"))

	// Create downloader
	downloader := communities.NewCodexIndexDownloader(suite.mockClient, testCid, filePath, suite.cancelChan, suite.logger)

	// Start download
	downloader.DownloadIndexFile()

	// Wait a bit for the goroutine to run
	time.Sleep(200 * time.Millisecond)

	// Verify that no bytes were recorded on error
	assert.Equal(suite.T(), int64(0), downloader.BytesCompleted(), "No bytes should be recorded on error")
	suite.T().Log("✅ Error handling: no bytes recorded on download failure")

	// File should exist but be empty (or not exist if creation failed)
	// This is current behavior - we might want to improve it to clean up on error
	if stat, err := os.Stat(filePath); err == nil {
		suite.T().Logf("File exists with size: %d bytes (current behavior)", stat.Size())
	} else {
		suite.T().Log("File does not exist (current behavior)")
	}
}

func (suite *CodexIndexDownloaderTestSuite) TestLength_ReturnsDatasetSize() {
	testCid := "zDvZRwzmTestCID123"
	filePath := filepath.Join(suite.testDir, "index.bin")
	expectedSize := int64(4096)

	// Setup mock to return a manifest
	expectedManifest := &communities.CodexManifest{
		CID: testCid,
	}
	expectedManifest.Manifest.DatasetSize = expectedSize

	suite.mockClient.EXPECT().
		FetchManifestWithContext(gomock.Any(), testCid).
		Return(expectedManifest, nil)

	// Create downloader
	downloader := communities.NewCodexIndexDownloader(suite.mockClient, testCid, filePath, suite.cancelChan, suite.logger)

	// Initially, Length should return 0
	assert.Equal(suite.T(), int64(0), downloader.Length(), "Initial length should be 0")

	// Fetch manifest
	manifestChan := downloader.GotManifest()
	<-manifestChan

	// Now Length should return the dataset size
	assert.Equal(suite.T(), expectedSize, downloader.Length(), "Length should return dataset size")
	suite.T().Logf("✅ Length() correctly returns dataset size: %d", downloader.Length())
}
