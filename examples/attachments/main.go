// Command attachments scans and downloads every attachment on one email.
//
//	export TROPMAIL_API_KEY=...
//	go run ./examples/attachments <mailbox-id> <email-id> ./downloads
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
	if len(os.Args) < 3 {
		log.Fatal("usage: attachments <mailbox-id> <email-id> [directory]")
	}
	mailboxID := os.Args[1]
	emailID := os.Args[2]
	directory := "."
	if len(os.Args) > 3 {
		directory = os.Args[3]
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
	scans, err := client.Emails.ScanAttachments(ctx, mailboxID, emailID)
	if err != nil {
		log.Fatal(err)
	}
	for _, scan := range scans {
		fmt.Printf("%-40s %s\n", scan.Filename, scan.ScanStatus)
	}

	items, err := client.Emails.DownloadAttachments(ctx, mailboxID, emailID)
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
		written, err := client.Attachments.DownloadTo(ctx, mailboxID, item.AttachmentID, target)
		if err != nil {
			log.Fatalf("download %s: %v", item.AttachmentID, err)
		}
		fmt.Printf("saved %s (%d bytes)\n", target, written)
	}
}
