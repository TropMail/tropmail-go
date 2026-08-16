package tropmail

import (
	"context"
	"net/http"
)

// MailboxesService covers GET /mailboxes and GET /mailboxes/{id}.
type MailboxesService struct {
	client *Client
}

// List returns every inbox the API key may see.
// Empty key scope means all current and future inboxes for the account.
func (s *MailboxesService) List(ctx context.Context) (*MailboxList, error) {
	var out MailboxList
	err := s.client.do(ctx, request{method: http.MethodGet, path: "/mailboxes"}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns the summary for one mailbox UUID.
func (s *MailboxesService) Get(ctx context.Context, mailboxID string) (*Mailbox, error) {
	path, err := mailboxPath(mailboxID)
	if err != nil {
		return nil, err
	}
	var out Mailbox
	if err := s.client.do(ctx, request{method: http.MethodGet, path: path}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Health is a liveness probe. It requires no authentication.
func (c *Client) Health(ctx context.Context) (*Health, error) {
	var out Health
	err := c.do(
		ctx,
		request{method: http.MethodGet, path: "/health", noAuth: true},
		&out,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
