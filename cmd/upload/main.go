package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"

	"go-codex-client/communities" // Import the local communities package
)

func main() {
	var (
		host     = flag.String("host", "localhost", "Codex host")
		port     = flag.String("port", "8080", "Codex port")
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

	fmt.Printf("Uploading %s (%d bytes) to Codex at %s:%s...\n", *file, len(data), *host, *port)
	// Create Codex client and upload
	client := communities.NewCodexClient(*host, *port)

	cid, err := client.Upload(bytes.NewReader(data), uploadName)
	if err != nil {
		log.Fatalf("Upload failed: %v", err)
	}

	fmt.Printf("✅ Upload successful!\n")
	fmt.Printf("CID: %s\n", cid)
}
