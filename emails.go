package tropmail

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/url"
)

const (
	defaultPageSize = 10
	iteratePageSize = 100
)

// ErrNoUpdateFields is returned when Update is called without any change.
var ErrNoUpdateFields = errors.New("tropmail: provide at least one of EmailState or ActionStatus")

// EmailsService covers /emails and /email/{id}.
type EmailsService struct {
	client *Client
}

// ListOptions filters and paginates the email list.
type ListOptions struct {
	Limit  int
	Page   int
	Status ListStatus
}

func (o ListOptions) body() map[string]any {
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
	return map[string]any{"limit": limit, "page": page, "status": status}
}

// List returns one page of emails.
func (s *EmailsService) List(ctx context.Context, opts ListOptions) (*EmailList, error) {
	var out EmailList
	err := s.client.do(
		ctx,
		request{method: http.MethodPost, path: "/emails", body: opts.body()},
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
	body := opts.body()
	delete(body, "status")
	body["query"] = query

	var out EmailList
	err := s.client.do(
		ctx,
		request{method: http.MethodPost, path: "/emails/search", body: body},
		&out,
	)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// All returns an iterator over every email, paging automatically.
//
// It stops on the first short page: Total counts the whole mailbox and cannot
// be used to detect the end of a filtered result set. Iteration halts on the
// first error, which is yielded alongside a zero Email.
//
//	for email, err := range client.Emails.All(ctx, tropmail.ListOptions{Status: "Open"}) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(email.Subject)
//	}
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
			result, err := s.List(ctx, ListOptions{Limit: limit, Page: page, Status: opts.Status})
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
			result, err := s.Search(ctx, query, ListOptions{Limit: limit, Page: page})
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
	// Supplying it makes the lookup faster.
	Timestamp string
}

func detailPath(id string, view View) string {
	if view == "" || view == ViewHTML {
		return "/email/" + url.PathEscape(id)
	}
	return "/email/" + url.PathEscape(id) + "/" + string(view)
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
	id string,
	opts GetOptions,
) (*EmailDetail, error) {
	var out EmailDetail
	err := s.client.do(ctx, request{
		method: http.MethodGet,
		path:   detailPath(id, opts.View),
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
	email Email,
	view View,
) (*EmailDetail, error) {
	return s.Get(ctx, email.ID, GetOptions{View: view, Timestamp: email.Timestamp})
}

// GetMarkdown fetches the markdown view, retrying the 504 the conversion returns.
//
// The server blocks up to ~60s while converting, then answers 504 and expects
// the same request to be retried.
func (s *EmailsService) GetMarkdown(
	ctx context.Context,
	id string,
	timestamp string,
) (*EmailDetail, error) {
	attempts := s.client.maxRetries + 1
	var lastErr error

	for attempt := 0; attempt < attempts; attempt++ {
		var out EmailDetail
		err := s.client.do(ctx, request{
			method:  http.MethodGet,
			path:    detailPath(id, ViewMarkdown),
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
//
// ActionStatus is a pointer so the empty string can be sent to clear a previous
// action, which also unblocks the sender.
type UpdateOptions struct {
	EmailState   EmailState
	ActionStatus *ActionStatus
	Timestamp    string
}

// Update changes an email's state and/or action status.
func (s *EmailsService) Update(
	ctx context.Context,
	id string,
	opts UpdateOptions,
) (*ActionResult, error) {
	if opts.EmailState == "" && opts.ActionStatus == nil {
		return nil, ErrNoUpdateFields
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
	err := s.client.do(ctx, request{
		method:  http.MethodPost,
		path:    "/email/" + url.PathEscape(id),
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
	id string,
	state EmailState,
) (*ActionResult, error) {
	return s.Update(ctx, id, UpdateOptions{EmailState: state})
}

// SetAction applies an action status. Pass [ActionNone] to clear it.
func (s *EmailsService) SetAction(
	ctx context.Context,
	id string,
	action ActionStatus,
) (*ActionResult, error) {
	return s.Update(ctx, id, UpdateOptions{ActionStatus: &action})
}

// Favorite flags an email as a favorite.
func (s *EmailsService) Favorite(ctx context.Context, id string) (*ActionResult, error) {
	return s.SetAction(ctx, id, ActionFavorite)
}

// Block blocks the sender and stops future inbound mail from them.
func (s *EmailsService) Block(ctx context.Context, id string) (*ActionResult, error) {
	return s.SetAction(ctx, id, ActionBlock)
}

// Delete soft-deletes an email.
func (s *EmailsService) Delete(ctx context.Context, id string) (*ActionResult, error) {
	return s.SetAction(ctx, id, ActionDelete)
}

// ClearAction clears any action status, unblocking the sender if blocked.
func (s *EmailsService) ClearAction(ctx context.Context, id string) (*ActionResult, error) {
	return s.SetAction(ctx, id, ActionNone)
}

// ScanAttachments kicks off scans for every unscanned attachment on an email.
func (s *EmailsService) ScanAttachments(ctx context.Context, id string) ([]Scan, error) {
	var out []Scan
	err := s.client.do(ctx, request{
		method:  http.MethodPost,
		path:    fmt.Sprintf("/email/%s/scan-attachments", url.PathEscape(id)),
		noRetry: true,
	}, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DownloadAttachments lists attachments on an email for authenticated download.
// Use Attachments.DownloadTo / Open with each AttachmentID to fetch bytes.
func (s *EmailsService) DownloadAttachments(ctx context.Context, id string) ([]Download, error) {
	var out []Download
	err := s.client.do(ctx, request{
		method: http.MethodGet,
		path:   fmt.Sprintf("/email/%s/download-attachments", url.PathEscape(id)),
	}, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}
