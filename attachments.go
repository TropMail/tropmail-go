package tropmail

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

// AttachmentsService covers /mailboxes/{id}/attachments/{attId}.
type AttachmentsService struct {
	client *Client
}

func attachmentPath(mailboxID, attachmentID string, extra ...string) (string, error) {
	parts := []string{"attachments", url.PathEscape(attachmentID)}
	parts = append(parts, extra...)
	return mailboxPath(mailboxID, parts...)
}

// Get fetches attachment metadata.
func (s *AttachmentsService) Get(ctx context.Context, mailboxID, attachmentID string) (*Attachment, error) {
	path, err := attachmentPath(mailboxID, attachmentID)
	if err != nil {
		return nil, err
	}
	var out Attachment
	err = s.client.do(ctx, request{
		method: http.MethodGet,
		path:   path,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Scan triggers a malware scan, or returns the cached result when already scanned.
func (s *AttachmentsService) Scan(ctx context.Context, mailboxID, attachmentID string) (*Scan, error) {
	path, err := attachmentPath(mailboxID, attachmentID, "scan")
	if err != nil {
		return nil, err
	}
	var out Scan
	err = s.client.do(ctx, request{
		method:  http.MethodPost,
		path:    path,
		noRetry: true,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Open streams attachment bytes from GET .../download (Bearer auth).
// The caller owns the returned reader and must close it.
func (s *AttachmentsService) Open(ctx context.Context, mailboxID, attachmentID string) (io.ReadCloser, error) {
	path, err := attachmentPath(mailboxID, attachmentID, "download")
	if err != nil {
		return nil, err
	}
	return s.client.openBinary(ctx, request{
		method: http.MethodGet,
		path:   path,
	})
}

// DownloadTo streams an attachment to path and returns the bytes written.
func (s *AttachmentsService) DownloadTo(
	ctx context.Context,
	mailboxID string,
	attachmentID string,
	path string,
) (int64, error) {
	body, err := s.Open(ctx, mailboxID, attachmentID)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return 0, fmt.Errorf("tropmail: create directory: %w", err)
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return 0, fmt.Errorf("tropmail: create file: %w", err)
	}
	defer file.Close()

	written, err := io.Copy(file, body)
	if err != nil {
		return written, fmt.Errorf("tropmail: write attachment: %w", err)
	}
	return written, nil
}
