// Command triage walks the whole mailbox, favorites anything matching a term,
// and blocks senders whose mail the API flagged as phishing.
//
//	export TROPMAIL_API_KEY=...
//	go run ./examples/triage invoice
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	tropmail "github.com/tropmail/tropmail-go"
)

func main() {
	term := "invoice"
	if len(os.Args) > 1 {
		term = os.Args[1]
	}

	client, err := tropmail.New("")
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	// The iterator pages for you and stops on the first short page. Requests
	// are paced to the tier's rate limit, so a full walk never collects 429s.
	favorited, blocked := 0, 0
	for email, err := range client.Emails.All(ctx, tropmail.ListOptions{}) {
		if err != nil {
			log.Fatal(err)
		}

		switch {
		case email.ActionStatus != nil && *email.ActionStatus == tropmail.ActionPhishing:
			if _, err := client.Emails.Block(ctx, email.ID); err != nil {
				log.Fatalf("block %s: %v", email.ID, err)
			}
			blocked++

		case strings.Contains(strings.ToLower(email.Subject), strings.ToLower(term)):
			if _, err := client.Emails.Favorite(ctx, email.ID); err != nil {
				// A message deleted mid-walk is expected, not fatal.
				if tropmail.IsNotFound(err) {
					continue
				}
				log.Fatalf("favorite %s: %v", email.ID, err)
			}
			favorited++
		}
	}

	fmt.Printf("favorited %d matching %q, blocked %d phishing senders\n",
		favorited, term, blocked)

	if _, err := client.Mailbox.Get(ctx); err != nil {
		var apiErr *tropmail.Error
		if errors.As(err, &apiErr) && apiErr.Status == 429 {
			fmt.Println("Rate limited; retry after", apiErr.RetryAfter)
		}
	}
}
