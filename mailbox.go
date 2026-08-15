package tropmail

import (
	"context"
	"net/http"
)

// MailboxService covers /mailbox, /validate and /health.
type MailboxService struct {
	client *Client
}

// Get returns the mailbox summary for the authenticated key.
func (s *MailboxService) Get(ctx context.Context) (*Mailbox, error) {
	var out Mailbox
	err := s.client.do(ctx, request{method: http.MethodGet, path: "/mailbox"}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Validate confirms the API key and reports its mailbox id and tier.
func (s *MailboxService) Validate(ctx context.Context) (*Validation, error) {
	var out Validation
	err := s.client.do(ctx, request{method: http.MethodGet, path: "/validate"}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Health is a liveness probe. It requires no authentication.
func (s *MailboxService) Health(ctx context.Context) (*Health, error) {
	var out Health
	err := s.client.do(
		ctx,
		request{method: http.MethodGet, path: "/health", noAuth: true},
		&out,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
