package tropmail

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/url"
	"strconv"
)

const (
	defaultPageSize = 10
	iteratePageSize = 100
)

// ErrNoUpdateFields is returned when Update is called without any change.
var ErrNoUpdateFields = errors.New("tropmail: provide at least one of EmailState or ActionStatus")

// EmailsService covers /mailboxes/{id}/emails.
type EmailsService struct {
	client *Client
}

// ListOptions filters and paginates the email list. MailboxID is required.
type ListOptions struct {
	MailboxID string
	Limit     int
	Page      int
	Status    ListStatus
}

func (o ListOptions) query() url.Values {
	limit := o.Limit
	if limit <= 0 {
		limit = defaultPageSize
	}
	page := o.Page
	if page < 1 {
		page = 1
	}
	status := o.Status
	if status == "" {
		status = StatusAll
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("page", strconv.Itoa(page))
	q.Set("status", string(status))
	return q
}

func (o ListOptions) emailsPath(extra ...string) (string, error) {
	parts := append([]string{"emails"}, extra...)
	return mailboxPath(o.MailboxID, parts...)
}

// List returns one page of emails.
func (s *EmailsService) List(ctx context.Context, opts ListOptions) (*EmailList, error) {
	path, err := opts.emailsPath()
	if err != nil {
		return nil, err
	}
	var out EmailList
	err = s.client.do(
		ctx,
		request{method: http.MethodGet, path: path, query: opts.query()},
		&out,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Search runs a full-text query. The API always reports Total as 0 for search.
func (s *EmailsService) Search(
	ctx context.Context,
	query string,
	opts ListOptions,
) (*EmailList, error) {
	path, err := opts.emailsPath("search")
	if err != nil {
		return nil, err
	}
	q := opts.query()
	q.Del("status")
	q.Set("query", query)

	var out EmailList
	err = s.client.do(
		ctx,
		request{method: http.MethodGet, path: path, query: q},
		&out,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// All returns an iterator over every email, paging automatically.
func (s *EmailsService) All(ctx context.Context, opts ListOptions) iter.Seq2[Email, error] {
	return func(yield func(Email, error) bool) {
		limit := opts.Limit
		if limit <= 0 {
			limit = iteratePageSize
		}
		page := opts.Page
		if page < 1 {
			page = 1
		}

		for {
			result, err := s.List(ctx, ListOptions{
				MailboxID: opts.MailboxID,
				Limit:     limit,
				Page:      page,
				Status:    opts.Status,
			})
			if err != nil {
				yield(Email{}, err)
				return
			}
			for _, email := range result.Emails {
				if !yield(email, nil) {
					return
				}
			}
			if len(result.Emails) < limit {
				return
			}
			page++
		}
	}
}

// SearchAll returns an iterator over every search hit, paging automatically.
func (s *EmailsService) SearchAll(
	ctx context.Context,
	query string,
	opts ListOptions,
) iter.Seq2[Email, error] {
	return func(yield func(Email, error) bool) {
		limit := opts.Limit
		if limit <= 0 {
			limit = iteratePageSize
		}

		for page := 1; ; page++ {
			result, err := s.Search(ctx, query, ListOptions{
				MailboxID: opts.MailboxID,
				Limit:     limit,
				Page:      page,
			})
			if err != nil {
				yield(Email{}, err)
				return
			}
			for _, email := range result.Emails {
				if !yield(email, nil) {
					return
				}
			}
			if len(result.Emails) < limit {
				return
			}
		}
	}
}

// GetOptions selects the body view and an optional email timestamp.
type GetOptions struct {
	View View
	// Timestamp is the email's RFC3339 timestamp from list/detail.
	Timestamp string
}

func detailPath(mailboxID, emailID string, view View) (string, error) {
	id := url.PathEscape(emailID)
	if view == "" || view == ViewHTML {
		return mailboxPath(mailboxID, "emails", id)
	}
	return mailboxPath(mailboxID, "emails", id, string(view))
}

func timestampQuery(ts string) url.Values {
	if ts == "" {
		return nil
	}
	return url.Values{"timestamp": []string{ts}}
}

// Get fetches a single email in the requested view, defaulting to HTML.
func (s *EmailsService) Get(
	ctx context.Context,
	mailboxID string,
	emailID string,
	opts GetOptions,
) (*EmailDetail, error) {
	path, err := detailPath(mailboxID, emailID, opts.View)
	if err != nil {
		return nil, err
	}
	var out EmailDetail
	err = s.client.do(ctx, request{
		method: http.MethodGet,
		path:   path,
		query:  timestampQuery(opts.Timestamp),
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetEmail fetches the detail for an email you already hold, forwarding its
// timestamp for a faster lookup.
func (s *EmailsService) GetEmail(
	ctx context.Context,
	mailboxID string,
	email Email,
	view View,
) (*EmailDetail, error) {
	return s.Get(ctx, mailboxID, email.ID, GetOptions{View: view, Timestamp: email.Timestamp})
}

// GetMarkdown fetches the markdown view, retrying the 504 the conversion returns.
func (s *EmailsService) GetMarkdown(
	ctx context.Context,
	mailboxID string,
	emailID string,
	timestamp string,
) (*EmailDetail, error) {
	attempts := s.client.maxRetries + 1
	var lastErr error

	for attempt := 0; attempt < attempts; attempt++ {
		path, err := detailPath(mailboxID, emailID, ViewMarkdown)
		if err != nil {
			return nil, err
		}
		var out EmailDetail
		err = s.client.do(ctx, request{
			method:  http.MethodGet,
			path:    path,
			query:   timestampQuery(timestamp),
			noRetry: true,
		}, &out)
		if err == nil {
			return &out, nil
		}
		if !IsMarkdownTimeout(err) {
			return nil, err
		}
		lastErr = err
		if attempt < attempts-1 {
			if sleepErr := sleepCtx(ctx, fullJitter(attempt)); sleepErr != nil {
				return nil, sleepErr
			}
		}
	}
	return nil, lastErr
}

// UpdateOptions describes an email state or action change.
type UpdateOptions struct {
	EmailState   EmailState
	ActionStatus *ActionStatus
	Timestamp    string
}

// Update changes an email's state and/or action status.
func (s *EmailsService) Update(
	ctx context.Context,
	mailboxID string,
	emailID string,
	opts UpdateOptions,
) (*ActionResult, error) {
	if opts.EmailState == "" && opts.ActionStatus == nil {
		return nil, ErrNoUpdateFields
	}

	path, err := mailboxPath(mailboxID, "emails", url.PathEscape(emailID))
	if err != nil {
		return nil, err
	}

	body := map[string]any{}
	if opts.EmailState != "" {
		body["email_state"] = opts.EmailState
	}
	if opts.ActionStatus != nil {
		body["action_status"] = *opts.ActionStatus
	}
	if opts.Timestamp != "" {
		body["timestamp"] = opts.Timestamp
	}

	var out ActionResult
	err = s.client.do(ctx, request{
		method:  http.MethodPost,
		path:    path,
		body:    body,
		noRetry: true,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetState marks an email opened or closed.
func (s *EmailsService) SetState(
	ctx context.Context,
	mailboxID string,
	emailID string,
	state EmailState,
) (*ActionResult, error) {
	return s.Update(ctx, mailboxID, emailID, UpdateOptions{EmailState: state})
}

// SetAction applies an action status. Pass [ActionNone] to clear it.
func (s *EmailsService) SetAction(
	ctx context.Context,
	mailboxID string,
	emailID string,
	action ActionStatus,
) (*ActionResult, error) {
	return s.Update(ctx, mailboxID, emailID, UpdateOptions{ActionStatus: &action})
}

// Favorite flags an email as a favorite.
func (s *EmailsService) Favorite(ctx context.Context, mailboxID, emailID string) (*ActionResult, error) {
	return s.SetAction(ctx, mailboxID, emailID, ActionFavorite)
}

// Block blocks the sender and stops future inbound mail from them.
func (s *EmailsService) Block(ctx context.Context, mailboxID, emailID string) (*ActionResult, error) {
	return s.SetAction(ctx, mailboxID, emailID, ActionBlock)
}

// Delete soft-deletes an email.
func (s *EmailsService) Delete(ctx context.Context, mailboxID, emailID string) (*ActionResult, error) {
	return s.SetAction(ctx, mailboxID, emailID, ActionDelete)
}

// ClearAction clears any action status, unblocking the sender if blocked.
func (s *EmailsService) ClearAction(ctx context.Context, mailboxID, emailID string) (*ActionResult, error) {
	return s.SetAction(ctx, mailboxID, emailID, ActionNone)
}

// ScanAttachments kicks off scans for every unscanned attachment on an email.
func (s *EmailsService) ScanAttachments(ctx context.Context, mailboxID, emailID string) ([]Scan, error) {
	path, err := mailboxPath(mailboxID, "emails", url.PathEscape(emailID), "scan-attachments")
	if err != nil {
		return nil, err
	}
	var out []Scan
	err = s.client.do(ctx, request{
		method:  http.MethodPost,
		path:    path,
		noRetry: true,
	}, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DownloadAttachments lists attachments on an email for authenticated download.
func (s *EmailsService) DownloadAttachments(ctx context.Context, mailboxID, emailID string) ([]Download, error) {
	path, err := mailboxPath(mailboxID, "emails", url.PathEscape(emailID), "download-attachments")
	if err != nil {
		return nil, err
	}
	var out []Download
	err = s.client.do(ctx, request{
		method: http.MethodGet,
		path:   path,
	}, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}
