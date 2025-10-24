package communities

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

// CodexIndexDownloader handles downloading index files from Codex storage
type CodexIndexDownloader struct {
	codexClient      CodexClientInterface
	indexCid         string
	filePath         string
	datasetSize      int64           // stores the dataset size from the manifest
	bytesCompleted   int64           // tracks download progress
	downloadComplete bool            // true when file is fully downloaded and renamed
	downloadError    error           // stores the last error that occurred during manifest fetch or download
	cancelChan       <-chan struct{} // for cancellation support
	logger           *zap.Logger
}

// NewCodexIndexDownloader creates a new index downloader
func NewCodexIndexDownloader(codexClient CodexClientInterface, indexCid string, filePath string, cancelChan <-chan struct{}, logger *zap.Logger) *CodexIndexDownloader {
	return &CodexIndexDownloader{
		codexClient: codexClient,
		indexCid:    indexCid,
		filePath:    filePath,
		cancelChan:  cancelChan,
		logger:      logger,
	}
}

// GotManifest returns a channel that is closed when the Codex manifest file
// for the configured CID is successfully fetched. On error, the channel is not closed
// (allowing timeout to handle failures). Check GetDatasetSize() > 0 to verify success.
func (d *CodexIndexDownloader) GotManifest() <-chan struct{} {
	ch := make(chan struct{})

	go func() {
		// Reset datasetSize to 0 to indicate no successful fetch yet
		d.datasetSize = 0
		d.downloadError = nil

		// Check for cancellation before starting
		select {
		case <-d.cancelChan:
			return // Exit without closing channel - cancellation
		default:
		}

		// Create cancellable context for HTTP request
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Monitor for cancellation in separate goroutine
		go func() {
			select {
			case <-d.cancelChan:
				cancel()
			case <-ctx.Done():
			}
		}()

		// Fetch manifest from Codex
		manifest, err := d.codexClient.FetchManifestWithContext(ctx, d.indexCid)
		if err != nil {
			d.downloadError = err
			d.logger.Debug("failed to fetch manifest",
				zap.String("indexCid", d.indexCid),
				zap.Error(err))
			// Don't close channel on error - let timeout handle it
			// This is to fit better in the original status-go app
			return
		}

		// Verify that the CID matches our configured indexCid
		if manifest.CID != d.indexCid {
			d.downloadError = fmt.Errorf("manifest CID mismatch: expected %s, got %s", d.indexCid, manifest.CID)
			d.logger.Debug("manifest CID mismatch",
				zap.String("expected", d.indexCid),
				zap.String("got", manifest.CID))
			return
		}

		// Store the dataset size for later use - this indicates success
		d.datasetSize = manifest.Manifest.DatasetSize

		// Success! Close the channel to signal completion
		close(ch)
	}()

	return ch
}

// GetDatasetSize returns the dataset size from the last successfully fetched manifest
func (d *CodexIndexDownloader) GetDatasetSize() int64 {
	return d.datasetSize
}

// DownloadIndexFile starts downloading the index file from Codex and writes it to the configured file path
func (d *CodexIndexDownloader) DownloadIndexFile() {
	// Reset progress counter and completion flag
	d.bytesCompleted = 0
	d.downloadComplete = false
	d.downloadError = nil

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Monitor for cancellation in separate goroutine
	go func() {
		select {
		case <-d.cancelChan:
			cancel() // Cancel download immediately
		case <-ctx.Done():
			// Context already cancelled, nothing to do
		}
	}()

	// Start download in separate goroutine
	go func() {
		defer cancel() // Ensure context is cancelled when download completes or fails

		// Create a temporary file in the same directory as the target file
		// This ensures atomic rename works (same filesystem)
		tmpFile, err := os.CreateTemp(filepath.Dir(d.filePath), ".codex-download-*.tmp")
		if err != nil {
			d.downloadError = fmt.Errorf("failed to create temporary file: %w", err)
			d.logger.Debug("failed to create temporary file",
				zap.String("filePath", d.filePath),
				zap.Error(err))
			return
		}
		tmpPath := tmpFile.Name()
		defer func() {
			tmpFile.Close()
			// Clean up temporary file if it still exists (i.e., download failed)
			os.Remove(tmpPath)
		}()

		// Create a progress tracking writer
		progressWriter := &progressWriter{
			writer:    tmpFile,
			completed: &d.bytesCompleted,
		}

		// Use CodexClient to download and stream to temporary file with context for cancellation
		err = d.codexClient.DownloadWithContext(ctx, d.indexCid, progressWriter)
		if err != nil {
			d.downloadError = fmt.Errorf("failed to download index file: %w", err)
			d.logger.Debug("failed to download index file",
				zap.String("indexCid", d.indexCid),
				zap.String("filePath", d.filePath),
				zap.String("tmpPath", tmpPath),
				zap.Error(err))
			return
		}

		// Close the temporary file before renaming
		if err := tmpFile.Close(); err != nil {
			d.downloadError = fmt.Errorf("failed to close temporary file: %w", err)
			d.logger.Debug("failed to close temporary file",
				zap.String("tmpPath", tmpPath),
				zap.Error(err))
			return
		}

		// Atomically rename temporary file to final destination
		// This ensures we only have a complete file at filePath
		if err := os.Rename(tmpPath, d.filePath); err != nil {
			d.downloadError = fmt.Errorf("failed to rename temporary file to final destination: %w", err)
			d.logger.Debug("failed to rename temporary file to final destination",
				zap.String("tmpPath", tmpPath),
				zap.String("filePath", d.filePath),
				zap.Error(err))
			return
		}

		// Mark download as complete only after successful rename
		d.downloadComplete = true
	}()
}

// BytesCompleted returns the number of bytes downloaded so far
func (d *CodexIndexDownloader) BytesCompleted() int64 {
	return d.bytesCompleted
}

// IsDownloadComplete returns true when the file has been fully downloaded and saved to disk
func (d *CodexIndexDownloader) IsDownloadComplete() bool {
	return d.downloadComplete
}

// GetError returns the last error that occurred during manifest fetch or download, or nil if no error
func (d *CodexIndexDownloader) GetError() error {
	return d.downloadError
}

// Length returns the total dataset size (equivalent to torrent file length)
func (d *CodexIndexDownloader) Length() int64 {
	return d.datasetSize
}

// progressWriter wraps an io.Writer to track bytes written
type progressWriter struct {
	writer    io.Writer
	completed *int64
}

func (pw *progressWriter) Write(p []byte) (n int, err error) {
	n, err = pw.writer.Write(p)
	if n > 0 {
		*pw.completed += int64(n)
	}
	return n, err
}
