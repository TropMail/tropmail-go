// Command quickstart prints a mailbox summary and reads the newest message.
//
//	export TROPMAIL_API_KEY=...
//	go run ./examples/quickstart
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	tropmail "github.com/tropmail/tropmail-go"
)

func main() {
	// An empty key falls back to TROPMAIL_API_KEY, and is validated locally so
	// a malformed key fails before spending a round trip.
	client, err := tropmail.New("")
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	mailbox, err := client.Mailbox.Get(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s — %d open, %d closed, %d favorite\n\n",
		mailbox.Email, mailbox.OpenedCount, mailbox.ClosedCount, mailbox.FavoriteCount)

	page, err := client.Emails.List(ctx, tropmail.ListOptions{Limit: 5})
	if err != nil {
		log.Fatal(err)
	}
	if len(page.Emails) == 0 {
		fmt.Println("The mailbox is empty.")
		return
	}

	for _, email := range page.Emails {
		fmt.Printf("%-24s %-40s %s\n",
			email.From.Address, truncate(email.Subject, 40), email.Timestamp)
	}

	// Passing the email rather than its id forwards the timestamp for a
	// faster lookup.
	newest := page.Emails[0]
	detail, err := client.Emails.GetEmail(ctx, newest, tropmail.ViewText)
	if err != nil {
		var apiErr *tropmail.Error
		if errors.As(err, &apiErr) {
			log.Fatalf("read failed (%d, request %s): %s",
				apiErr.Status, apiErr.RequestID, apiErr.Message)
		}
		log.Fatal(err)
	}

	fmt.Printf("\n--- %s ---\n%s\n", detail.Subject, truncate(detail.Content, 500))
}

func truncate(text string, width int) string {
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	return string(runes[:width]) + "…"
}
