package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"path"

	"go-codex-client/codexclient"

	"github.com/codex-storage/codex-go-bindings/codex"
)

func main() {
	var (
		file     = flag.String("file", "test-data.bin", "File to upload")
		filename = flag.String("name", "", "Filename to use in upload (defaults to actual filename)")
	)
	flag.Parse()

	// Read file data
	data, err := os.ReadFile(*file)
	if err != nil {
		log.Fatalf("Failed to read file %s: %v", *file, err)
	}

	// Use actual filename if name not specified
	uploadName := *filename
	if uploadName == "" {
		uploadName = *file
	}

	fmt.Printf("Uploading %s (%d bytes) to Codex...\n", *file, len(data))
	// Create Codex client and upload
	client, err := codexclient.NewCodexClient(codex.Config{
		LogFormat:      codex.LogFormatNoColors,
		MetricsEnabled: false,
		BlockRetries:   5,
		LogLevel:       "ERROR",
		DataDir:        path.Join(os.TempDir(), "codex-client-data"),
	})
	if err != nil {
		log.Fatalf("Failed to create CodexClient: %v", err)
	}

	if err := client.Start(); err != nil {
		log.Fatalf("Failed to start CodexClient: %v", err)
	}

	cid, err := client.Upload(bytes.NewReader(data), uploadName)
	if err != nil {
		log.Fatalf("Upload failed: %v", err)
	}

	if err := client.Stop(); err != nil {
		log.Printf("Warning: Failed to stop CodexClient: %v", err)
	}
	if err := client.Destroy(); err != nil {
		log.Printf("Warning: Failed to stop CodexClient: %v", err)
	}

	fmt.Printf("✅ Upload successful!\n")
	fmt.Printf("CID: %s\n", cid)
}
