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

// AttachmentsService covers /attachment/{id}.
type AttachmentsService struct {
	client *Client
}

// Get fetches attachment metadata.
func (s *AttachmentsService) Get(ctx context.Context, id string) (*Attachment, error) {
	var out Attachment
	err := s.client.do(ctx, request{
		method: http.MethodGet,
		path:   "/attachment/" + url.PathEscape(id),
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Scan triggers a malware scan, or returns the cached result when already scanned.
func (s *AttachmentsService) Scan(ctx context.Context, id string) (*Scan, error) {
	var out Scan
	err := s.client.do(ctx, request{
		method:  http.MethodPost,
		path:    fmt.Sprintf("/attachment/%s/scan", url.PathEscape(id)),
		noRetry: true,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Open streams attachment bytes from GET /attachment/{id}/download (Bearer auth).
// The caller owns the returned reader and must close it.
func (s *AttachmentsService) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	return s.client.openBinary(ctx, request{
		method: http.MethodGet,
		path:   fmt.Sprintf("/attachment/%s/download", url.PathEscape(id)),
	})
}

// DownloadTo streams an attachment to path and returns the bytes written.
func (s *AttachmentsService) DownloadTo(
	ctx context.Context,
	id string,
	path string,
) (int64, error) {
	body, err := s.Open(ctx, id)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
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
