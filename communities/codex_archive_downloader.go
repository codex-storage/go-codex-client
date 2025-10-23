//go:build !disable_torrent
// +build !disable_torrent

package communities

import (
	"context"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"

	"go-codex-client/protobuf"
)

// CodexArchiveProcessor handles processing of downloaded archive data
type CodexArchiveProcessor interface {
	// ProcessArchiveData processes the raw archive data and returns any error
	// The processor is responsible for extracting messages, handling them,
	// and saving the archive ID to persistence
	ProcessArchiveData(communityID string, archiveHash string, archiveData []byte, from, to uint64) error
}

// CodexArchiveDownloader handles downloading individual archive files from Codex storage
type CodexArchiveDownloader struct {
	codexClient        CodexClientInterface
	index              *protobuf.CodexWakuMessageArchiveIndex
	communityID        string
	existingArchiveIDs []string
	cancelChan         chan struct{} // for cancellation support

	// Progress tracking
	totalArchivesCount           int
	totalDownloadedArchivesCount int
	currentArchiveHash           string
	archiveDownloadProgress      map[string]int64 // hash -> bytes downloaded
	archiveDownloadCancel        map[string]chan struct{}
	mu                           sync.RWMutex

	// Download control
	downloadComplete bool
	cancelled        bool
	pollingInterval  time.Duration // configurable polling interval for HasCid checks

	// Callbacks
	onArchiveDownloaded       func(hash string, from, to uint64)
	onStartingArchiveDownload func(hash string, from, to uint64)
}

// NewCodexArchiveDownloader creates a new archive downloader
func NewCodexArchiveDownloader(codexClient CodexClientInterface, index *protobuf.CodexWakuMessageArchiveIndex, communityID string, existingArchiveIDs []string, cancelChan chan struct{}) *CodexArchiveDownloader {
	return &CodexArchiveDownloader{
		codexClient:                  codexClient,
		index:                        index,
		communityID:                  communityID,
		existingArchiveIDs:           existingArchiveIDs,
		cancelChan:                   cancelChan,
		totalArchivesCount:           len(index.Archives),
		totalDownloadedArchivesCount: len(existingArchiveIDs),
		archiveDownloadProgress:      make(map[string]int64),
		archiveDownloadCancel:        make(map[string]chan struct{}),
		pollingInterval:              1 * time.Second, // Default production polling interval
	}
}

// SetPollingInterval sets the polling interval for HasCid checks (useful for testing)
func (d *CodexArchiveDownloader) SetPollingInterval(interval time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pollingInterval = interval
}

// SetOnArchiveDownloaded sets a callback function to be called when an archive is successfully downloaded
func (d *CodexArchiveDownloader) SetOnArchiveDownloaded(callback func(hash string, from, to uint64)) {
	d.onArchiveDownloaded = callback
}

// SetOnStartingArchiveDownload sets a callback function to be called before starting an archive download
// This callback is called on the main thread before launching goroutines, making it useful for testing
// the deterministic order in which archives are processed (sorted newest first)
func (d *CodexArchiveDownloader) SetOnStartingArchiveDownload(callback func(hash string, from, to uint64)) {
	d.onStartingArchiveDownload = callback
}

// GetTotalArchivesCount returns the total number of archives to download
func (d *CodexArchiveDownloader) GetTotalArchivesCount() int {
	return d.totalArchivesCount
}

// GetTotalDownloadedArchivesCount returns the number of archives already downloaded
func (d *CodexArchiveDownloader) GetTotalDownloadedArchivesCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.totalDownloadedArchivesCount
}

func (d *CodexArchiveDownloader) GetPendingArchivesCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.archiveDownloadCancel)
}

// GetCurrentArchiveHash returns the hash of the currently downloading archive
func (d *CodexArchiveDownloader) GetCurrentArchiveHash() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentArchiveHash
}

// GetArchiveDownloadProgress returns the download progress for a specific archive
func (d *CodexArchiveDownloader) GetArchiveDownloadProgress(hash string) int64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.archiveDownloadProgress[hash]
}

// IsDownloadComplete returns whether all archives have been downloaded
func (d *CodexArchiveDownloader) IsDownloadComplete() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.downloadComplete
}

// IsCancelled returns whether the download was cancelled
func (d *CodexArchiveDownloader) IsCancelled() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cancelled
}

// StartDownload begins downloading all missing archives
func (d *CodexArchiveDownloader) StartDownload() {
	d.downloadAllArchives()
}

// downloadAllArchives handles the main download loop for all archives
func (d *CodexArchiveDownloader) downloadAllArchives() {
	// Create sorted list of archives (newest first, like torrent version)
	type archiveInfo struct {
		hash string
		from uint64
		to   uint64
		cid  string
	}

	var archivesList []archiveInfo
	for hash, metadata := range d.index.Archives {
		archivesList = append(archivesList, archiveInfo{
			hash: hash,
			from: metadata.Metadata.From,
			to:   metadata.Metadata.To,
			cid:  metadata.Cid,
		})
	}

	// Sort by timestamp (newest first)
	slices.SortFunc(archivesList, func(a, b archiveInfo) int {
		if a.from > b.from {
			return -1 // a is newer, should come first
		}
		if a.from < b.from {
			return 1 // b is newer, should come first
		}
		return 0 // equal timestamps
	})

	// Monitor for cancellation in a separate goroutine
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-d.cancelChan:
				d.mu.Lock()
				for hash, cancelChan := range d.archiveDownloadCancel {
					select {
					case <-cancelChan:
						// Already closed
					default:
						close(cancelChan) // Safe to close
					}
					delete(d.archiveDownloadCancel, hash)
				}
				d.cancelled = true
				d.mu.Unlock()
				return // Exit goroutine after cancellation
			case <-ticker.C:
				// Check if downloads are complete
				d.mu.RLock()
				complete := d.downloadComplete
				d.mu.RUnlock()

				if complete {
					return // Exit goroutine when downloads complete
				}
			}
		}
	}()

	// Download each missing archive
	for _, archive := range archivesList {
		// Check if we already have this archive
		hasArchive := slices.Contains(d.existingArchiveIDs, archive.hash)
		if hasArchive {
			continue
		}

		archiveCancelChan := make(chan struct{})

		d.mu.Lock()
		d.currentArchiveHash = archive.hash
		d.archiveDownloadProgress[archive.hash] = 0
		d.archiveDownloadCancel[archive.hash] = archiveCancelChan
		d.mu.Unlock()

		// Call callback before starting
		if d.onStartingArchiveDownload != nil {
			d.onStartingArchiveDownload(archive.hash, archive.from, archive.to)
		}

		// Trigger archive download and track progress in a goroutine
		go func(archiveHash, archiveCid string, archiveFrom, archiveTo uint64, archiveCancel chan struct{}) {
			err := d.triggerSingleArchiveDownload(archiveHash, archiveCid, archiveCancel)

			// Update shared state with minimal lock scope
			d.mu.Lock()
			if err == nil {
				d.totalDownloadedArchivesCount++
			}
			d.mu.Unlock()

			// poll at configured interval until we confirm it's downloaded
			// or timeout after 30 seconds
			timeout := time.After(30 * time.Second)
			ticker := time.NewTicker(d.pollingInterval)
			defer ticker.Stop()
		PollLoop:
			for {
				select {
				case <-timeout:
					log.Printf("timeout waiting for CID %s to be available locally", archiveCid)
					break PollLoop
				case <-ticker.C:
					hasCid, err := d.codexClient.HasCid(archiveCid)
					if err != nil {
						// Log error but continue polling
						log.Printf("error checking CID %s: %v", archiveCid, err)
						continue
					}
					if hasCid {
						// CID is now available locally
						break PollLoop
					}
				}
			}

			// Update shared state with minimal lock scope
			d.mu.Lock()
			// Remove from active downloads
			delete(d.archiveDownloadCancel, archiveHash)

			// Check if all downloads are complete
			d.downloadComplete = len(d.archiveDownloadCancel) == 0
			d.mu.Unlock()

			// Call the callback outside the lock to avoid blocking other operations
			if d.onArchiveDownloaded != nil {
				d.onArchiveDownloaded(archiveHash, archiveFrom, archiveTo)
			}
		}(archive.hash, archive.cid, archive.from, archive.to, archiveCancelChan)
	}
}

// triggerSingleArchiveDownload downloads a single archive by its CID
func (d *CodexArchiveDownloader) triggerSingleArchiveDownload(hash, cid string, cancelChan <-chan struct{}) error {
	// Create a context that can be cancelled via our cancel channel
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Monitor for cancellation in a separate goroutine
	go func() {
		select {
		case <-cancelChan:
			cancel() // Cancel the download immediately
		case <-ctx.Done():
			// Context already cancelled, nothing to do
		}
	}()

	manifest, err := d.codexClient.TriggerDownloadWithContext(ctx, cid)
	if err != nil {
		return fmt.Errorf("failed to trigger archive download with CID %s: %w", cid, err)
	}

	if manifest.CID != cid {
		return fmt.Errorf("unexpected manifest CID %s, expected %s", manifest.CID, cid)
	}

	return nil
}
