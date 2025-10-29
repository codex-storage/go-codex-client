package main

import (
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
		cid  = flag.String("cid", "", "CID of the file to download")
		file = flag.String("file", "downloaded-file.bin", "File to save the downloaded data")
	)

	flag.Parse()

	if *cid == "" {
		log.Fatal("CID is required (use -cid flag)")
	}

	// Create Codex client
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

	// Create output file
	outputFile, err := os.Create(*file)
	if err != nil {
		log.Fatalf("Failed to create output file %s: %v", *file, err)
	}
	defer outputFile.Close()

	// Download data - pass the io.Writer (outputFile), not the string
	err = client.Download(*cid, outputFile)
	if err != nil {
		// Clean up the failed/partial file
		os.Remove(*file)
		log.Fatalf("Download failed: %v", err)
	}

	if err := client.Stop(); err != nil {
		log.Printf("Warning: Failed to stop CodexClient: %v", err)
	}
	if err := client.Destroy(); err != nil {
		log.Printf("Warning: Failed to stop CodexClient: %v", err)
	}

	fmt.Printf("✅ Download successful!\n")
	fmt.Printf("Saved to: %s\n", *file)
}
