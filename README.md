# tropmail-go

Official Go client for the [TropMail API](https://api.tropmail.com).

Standard library only — no third-party dependencies.

```bash
go get github.com/tropmail/tropmail-go
```

Requires Go 1.23+ (for range-over-func iterators). The client reads `TROPMAIL_API_KEY` when you pass an empty key.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/tropmail/tropmail-go"
)

func main() {
	client, err := tropmail.New("") // reads TROPMAIL_API_KEY
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	listed, err := client.Mailboxes.List(ctx)
	if err != nil {
		log.Fatal(err)
	}
	mailbox := listed.Mailboxes[0]
	fmt.Printf("%s: %d opened\n", mailbox.Email, mailbox.OpenedCount)

	for email, err := range client.Emails.All(ctx, tropmail.ListOptions{MailboxID: mailbox.ID, Status: "Open"}) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(email.Timestamp, email.From.Address, email.Subject)
	}
}
```

## Pagination

`All` and `SearchAll` return `iter.Seq2[Email, error]`, so they compose with
`range` and stop as soon as you `break`:

```go
for email, err := range client.Emails.SearchAll(ctx, "invoice", tropmail.ListOptions{MailboxID: mailbox.ID}) {
	if err != nil {
		return err
	}
	if strings.Contains(email.Subject, "Q4") {
		return handle(email)
	}
}
```

The iterators stop on the first short page. `Total` on list is the mailbox
opened count plus closed count. `Total` on search is the match count.

## Reading an email

`Get` returns the HTML view by default. Use `GetEmail` with an email you already
have and the client forwards its timestamp for a faster lookup.

```go
page, err := client.Emails.List(ctx, tropmail.ListOptions{MailboxID: mailbox.ID, Limit: 10})
if err != nil {
	return err
}

detail, err := client.Emails.GetEmail(ctx, mailbox.ID, page.Emails[0], tropmail.ViewText)
if err != nil {
	return err
}
fmt.Println(detail.Content)
```

### Markdown

The markdown view is generated on demand and the server can block for up to a
minute before answering `504`. `GetMarkdown` handles that retry:

```go
detail, err := client.Emails.GetMarkdown(ctx, mailbox.ID, email.ID, email.Timestamp)
```

## Actions

```go
client.Emails.Favorite(ctx, mailbox.ID, id)
client.Emails.SetState(ctx, mailbox.ID, id, tropmail.StateClose)
client.Emails.Block(ctx, mailbox.ID, id)       // also blocks the sender
client.Emails.ClearAction(ctx, mailbox.ID, id) // clears it, unblocking the sender
```

## Attachments

```go
info, err := client.Attachments.Get(ctx, mailbox.ID, attachmentID)
scan, err := client.Attachments.Scan(ctx, mailbox.ID, attachmentID)

n, err := client.Attachments.DownloadTo(ctx, mailbox.ID, attachmentID, "/tmp/invoice.pdf")

// Or stream it yourself.
body, err := client.Attachments.Open(ctx, mailbox.ID, attachmentID)
defer body.Close()
```

Downloads use your API key on the same host as the rest of the API.

## Errors

Every API failure is a `*tropmail.Error` carrying the status, message, and the
`X-Request-ID` the API echoed back, which is what support needs to trace a call.

```go
detail, err := client.Emails.Get(ctx, mailbox.ID, id, tropmail.GetOptions{})
switch {
case tropmail.IsNotFound(err):
	// gone
case tropmail.IsRateLimit(err):
	var apiErr *tropmail.Error
	errors.As(err, &apiErr)
	time.Sleep(apiErr.RetryAfter)
case err != nil:
	return err
}
```

| Helper | Condition |
|---|---|
| `IsValidation` | 400 |
| `IsAuth` | 401 |
| `IsTier` | 403 |
| `IsNotFound` | 404 |
| `IsRateLimit` | 429 |
| `IsMarkdownTimeout` | 504 |
| `IsServer` | 5xx except 504 |
| `IsConnection` | transport failure |

## Rate limits

Budgets are per account, per second: Pro 3, Ultimate 10, Enterprise 50.

The client reads the limit from response headers and stays within your
account budget:

```go
snap := client.RateLimit()
fmt.Println(snap.Limit, snap.Remaining, snap.Reset)
```

Pass `tropmail.WithThrottle(false)` if you manage concurrency yourself.

## Configuration

```go
client, err := tropmail.New(apiKey,
	tropmail.WithBaseURL("https://api.tropmail.com/api/v1"),
	tropmail.WithTimeout(120*time.Second), // markdown can block ~60s
	tropmail.WithMaxRetries(3),
	tropmail.WithThrottle(true),
	tropmail.WithHTTPClient(myClient),
	tropmail.WithUserAgent("my-app/1.0"),
)
```

Every call takes a
`context.Context`. Retries with backoff on 429, 502, 503, 504, and transport
errors. Reads retry automatically; mutations never do.

## Examples

Runnable programs live in [`examples/`](examples/): a quickstart, a bulk triage
walk, and attachment downloading.

## Development

```bash
make test
make lint
make contract
```

## Docs

Guides and API reference: [docs.tropmail.com](https://docs.tropmail.com/sdks/go/).

## License

MIT
