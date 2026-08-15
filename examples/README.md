# Go examples

```bash
export TROPMAIL_API_KEY=your32charalphanumericapikeyhere
```

| Directory | What it shows |
|---|---|
| [`quickstart`](quickstart/) | mailbox summary, listing, reading one message, `errors.As` on `*tropmail.Error` |
| [`triage`](triage/) | `iter.Seq2` auto-paging, bulk actions, tolerating a message deleted mid-walk |
| [`attachments`](attachments/) | scanning attachments and streaming them to disk |

```bash
go run ./examples/quickstart
go run ./examples/triage invoice
go run ./examples/attachments <email-id> ./downloads
```

These are part of the module, so `go vet ./...` and `go build ./...` compile
them in CI and they cannot drift from the SDK.
