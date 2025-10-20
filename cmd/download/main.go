package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"go-codex-client/communities" // Import the local communities package
)

func main() {
	var (
		host = flag.String("host", "localhost", "Codex host")
		port = flag.String("port", "8080", "Codex port")
		cid  = flag.String("cid", "", "CID of the file to download")
		file = flag.String("file", "downloaded-file.bin", "File to save the downloaded data")
	)

	flag.Parse()

	if *cid == "" {
		log.Fatal("CID is required (use -cid flag)")
	}

	// Create Codex client
	client := communities.NewCodexClient(*host, *port)

	// Create output file
	outputFile, err := os.Create(*file)
	if err != nil {
		log.Fatalf("Failed to create output file %s: %v", *file, err)
	}
	defer outputFile.Close()

	fmt.Printf("Downloading CID %s from Codex at %s:%s...\n", *cid, *host, *port)

	// Download data - pass the io.Writer (outputFile), not the string
	err = client.Download(*cid, outputFile)
	if err != nil {
		// Clean up the failed/partial file
		os.Remove(*file)
		log.Fatalf("Download failed: %v", err)
	}

	fmt.Printf("✅ Download successful!\n")
	fmt.Printf("Saved to: %s\n", *file)
}
