package main

import (
	"go-codex-client/communities"
	"go-codex-client/protobuf"
	"go.uber.org/zap"
)

func main() {
	// Create a production logger with JSON output
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Or create a development logger with console output
	// logger, _ := zap.NewDevelopment()

	// Example usage in production
	var (
		codexClient       communities.CodexClientInterface = nil // your actual client
		index             *protobuf.CodexWakuMessageArchiveIndex = nil // your index
		communityID       = "your-community-id"
		existingArchives  = []string{"existing-hash-1", "existing-hash-2"}
		cancelChan        = make(chan struct{})
	)

	// Create downloader with structured logging
	downloader := communities.NewCodexArchiveDownloader(
		codexClient,
		index,
		communityID,
		existingArchives,
		cancelChan,
		logger,
	)

	// Configure for production use
	downloader.SetOnArchiveDownloaded(func(hash string, from, to uint64) {
		logger.Info("archive download completed",
			zap.String("hash", hash),
			zap.String("community", communityID),
			zap.Uint64("from", from),
			zap.Uint64("to", to))
	})

	downloader.SetOnStartingArchiveDownload(func(hash string, from, to uint64) {
		logger.Info("starting archive download",
			zap.String("hash", hash),
			zap.String("community", communityID),
			zap.Uint64("from", from),
			zap.Uint64("to", to))
	})

	// Start downloads - all internal logging will now use structured zap logging
	// downloader.StartDownload()
}