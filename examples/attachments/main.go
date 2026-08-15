// Command attachments scans and downloads every attachment on one email.
//
//	export TROPMAIL_API_KEY=...
//	go run ./examples/attachments <email-id> ./downloads
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	tropmail "github.com/tropmail/tropmail-go"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: attachments <email-id> [directory]")
	}
	emailID := os.Args[1]
	directory := "."
	if len(os.Args) > 2 {
		directory = os.Args[2]
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		log.Fatal(err)
	}

	client, err := tropmail.New("")
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	// Scanning is asynchronous: a fresh request comes back as Processing and
	// the verdict lands on a later read.
	scans, err := client.Emails.ScanAttachments(ctx, emailID)
	if err != nil {
		log.Fatal(err)
	}
	for _, scan := range scans {
		fmt.Printf("%-40s %s\n", scan.Filename, scan.ScanStatus)
	}

	items, err := client.Emails.DownloadAttachments(ctx, emailID)
	if err != nil {
		log.Fatal(err)
	}
	for _, item := range items {
		if !item.Available {
			fmt.Printf("skip %s (not available)\n", item.Filename)
			continue
		}
		// filepath.Base keeps a server-supplied name from escaping the directory.
		target := filepath.Join(directory, filepath.Base(item.Filename))
		written, err := client.Attachments.DownloadTo(ctx, item.AttachmentID, target)
		if err != nil {
			log.Fatalf("download %s: %v", item.AttachmentID, err)
		}
		fmt.Printf("saved %s (%d bytes)\n", target, written)
	}
}
