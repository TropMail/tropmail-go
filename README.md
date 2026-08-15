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
	mailbox, err := client.Mailbox.Get(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %d opened\n", mailbox.Email, mailbox.OpenedCount)

	for email, err := range client.Emails.All(ctx, tropmail.ListOptions{Status: "Open"}) {
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
for email, err := range client.Emails.SearchAll(ctx, "invoice", tropmail.ListOptions{}) {
	if err != nil {
		return err
	}
	if strings.Contains(email.Subject, "Q4") {
		return handle(email)
	}
}
```

The iterators stop on the first short page. `Total` counts the whole mailbox on
list calls and is always `0` for search, so it cannot end a filtered scan.

## Reading an email

`Get` returns the HTML view by default. Use `GetEmail` with an email you already
have and the client forwards its timestamp for a faster lookup.

```go
page, err := client.Emails.List(ctx, tropmail.ListOptions{Limit: 10})
if err != nil {
	return err
}

detail, err := client.Emails.GetEmail(ctx, page.Emails[0], tropmail.ViewText)
if err != nil {
	return err
}
fmt.Println(detail.Content)
```

### Markdown

The markdown view is generated on demand and the server can block for up to a
minute before answering `504`. `GetMarkdown` handles that retry:

```go
detail, err := client.Emails.GetMarkdown(ctx, email.ID, email.Timestamp)
```

## Actions

```go
client.Emails.Favorite(ctx, id)
client.Emails.SetState(ctx, id, tropmail.StateClose)
client.Emails.Block(ctx, id)       // also blocks the sender
client.Emails.ClearAction(ctx, id) // clears it, unblocking the sender
```

## Attachments

```go
info, err := client.Attachments.Get(ctx, attachmentID)
scan, err := client.Attachments.Scan(ctx, attachmentID)

n, err := client.Attachments.DownloadTo(ctx, attachmentID, "/tmp/invoice.pdf")

// Or stream it yourself.
body, err := client.Attachments.Open(ctx, attachmentID)
defer body.Close()
```

The download URL lives on a separate host, so no API credentials are sent to it.

## Errors

Every API failure is a `*tropmail.Error` carrying the status, message, and the
`X-Request-ID` the API echoed back, which is what support needs to trace a call.

```go
detail, err := client.Emails.Get(ctx, id, tropmail.GetOptions{})
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

Budgets are per mailbox, per second: Pro 3, Ultimate 10, Enterprise 50.

The client reads the limit from response headers and paces itself with a token
bucket so you stay under the budget instead of collecting `429`s:

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

The default HTTP client uses a transport tuned for a single hot API host
(HTTP/2, 10 idle connections per host, 90s idle timeout). Every call takes a
`context.Context`; retries use exponential backoff with full jitter on 429, 502,
503, 504, and transport errors. Reads retry automatically; mutations never do.

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
