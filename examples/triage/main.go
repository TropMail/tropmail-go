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

	listed, err := client.Mailboxes.List(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if len(listed.Mailboxes) == 0 {
		log.Fatal("this API key has no mailboxes")
	}
	mailboxID := listed.Mailboxes[0].ID

	favorited, blocked := 0, 0
	for email, err := range client.Emails.All(ctx, tropmail.ListOptions{MailboxID: mailboxID}) {
		if err != nil {
			log.Fatal(err)
		}

		switch {
		case email.ActionStatus != nil && *email.ActionStatus == tropmail.ActionPhishing:
			if _, err := client.Emails.Block(ctx, mailboxID, email.ID); err != nil {
				log.Fatalf("block %s: %v", email.ID, err)
			}
			blocked++

		case strings.Contains(strings.ToLower(email.Subject), strings.ToLower(term)):
			if _, err := client.Emails.Favorite(ctx, mailboxID, email.ID); err != nil {
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

	if _, err := client.Mailboxes.Get(ctx, mailboxID); err != nil {
		var apiErr *tropmail.Error
		if errors.As(err, &apiErr) && apiErr.Status == 429 {
			fmt.Println("Rate limited; retry after", apiErr.RetryAfter)
		}
	}
}
