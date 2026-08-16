package tropmail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testAPIKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const testMailboxID = "550e8400-e29b-41d4-a716-446655440000"

type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   map[string]any
}

// testServer spins up an httptest server and a client wired to it.
func testServer(t *testing.T, handler http.HandlerFunc, opts ...Option) (*Client, *[]recordedRequest) {
	t.Helper()

	var recorded []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Header: r.Header.Clone(),
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&rec.Body)
		}
		recorded = append(recorded, rec)
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	defaults := []Option{
		WithBaseURL(server.URL + "/api/v1"),
		WithThrottle(false),
		WithMaxRetries(0),
	}
	client, err := New(testAPIKey, append(defaults, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client, &recorded
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, status int, data any) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"success": true,
		"message": "ok",
		"data":    data,
		"error":   nil,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", "req-123")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	payload, _ := json.Marshal(map[string]any{
		"success": false,
		"message": message,
		"data":    nil,
		"error":   message,
	})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", "req-err")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func TestNewRejectsMalformedKey(t *testing.T) {
	t.Setenv("TROPMAIL_API_KEY", "")
	for _, key := range []string{"", "short", "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"} {
		if _, err := New(key); err == nil {
			t.Errorf("New(%q) should fail", key)
		}
	}
}

func TestNewReadsKeyFromEnvironment(t *testing.T) {
	t.Setenv("TROPMAIL_API_KEY", testAPIKey)
	if _, err := New(""); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestSendsBearerTokenAndRequestID(t *testing.T) {
	client, recorded := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, http.StatusOK, Mailbox{ID: "m1", Email: "a@b.dev"})
	})

	if _, err := client.Mailboxes.Get(context.Background(), testMailboxID); err != nil {
		t.Fatalf("Get: %v", err)
	}

	got := (*recorded)[0]
	if want := "Bearer " + testAPIKey; got.Header.Get("Authorization") != want {
		t.Errorf("Authorization = %q, want %q", got.Header.Get("Authorization"), want)
	}
	if got.Header.Get("X-Request-ID") == "" {
		t.Error("missing X-Request-ID")
	}
	if got.Path != "/api/v1/mailboxes/"+testMailboxID {
		t.Errorf("path = %q", got.Path)
	}
}

func TestHealthOmitsAuthorization(t *testing.T) {
	client, recorded := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, http.StatusOK, Health{Status: "ok", Version: "1.0.0"})
	})

	health, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if health.Status != "ok" {
		t.Errorf("status = %q", health.Status)
	}
	if auth := (*recorded)[0].Header.Get("Authorization"); auth != "" {
		t.Errorf("Authorization should be empty, got %q", auth)
	}
}

func TestUnwrapsEnvelope(t *testing.T) {
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, http.StatusOK, Mailbox{
			ID: "m1", Email: "user@tropmail.com", OpenedCount: 3, FavoriteCount: 1,
		})
	})

	mailbox, err := client.Mailboxes.Get(context.Background(), testMailboxID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if mailbox.Email != "user@tropmail.com" || mailbox.OpenedCount != 3 {
		t.Errorf("unexpected mailbox: %+v", mailbox)
	}
}

func TestListSendsQueryDefaults(t *testing.T) {
	client, recorded := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, http.StatusOK, EmailList{Limit: 10, Page: 1})
	})

	if _, err := client.Emails.List(context.Background(), ListOptions{MailboxID: testMailboxID}); err != nil {
		t.Fatalf("List: %v", err)
	}

	q := (*recorded)[0].Query
	if !strings.Contains(q, "limit=10") || !strings.Contains(q, "page=1") || !strings.Contains(q, "status=all") {
		t.Errorf("query = %q", q)
	}
}

func TestDetailPathAndTimestampForwarding(t *testing.T) {
	client, recorded := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, http.StatusOK, EmailDetail{ID: "abc"})
	})
	ctx := context.Background()

	if _, err := client.Emails.Get(ctx, testMailboxID, "abc", GetOptions{}); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := client.Emails.Get(ctx, testMailboxID, "abc", GetOptions{View: ViewText}); err != nil {
		t.Fatalf("Get text: %v", err)
	}
	email := Email{ID: "abc", Timestamp: "2026-01-01T00:00:00Z"}
	if _, err := client.Emails.GetEmail(ctx, testMailboxID, email, ViewHTML); err != nil {
		t.Fatalf("GetEmail: %v", err)
	}

	calls := *recorded
	if calls[0].Path != "/api/v1/mailboxes/"+testMailboxID+"/emails/abc" {
		t.Errorf("html path = %q", calls[0].Path)
	}
	if calls[1].Path != "/api/v1/mailboxes/"+testMailboxID+"/emails/abc/text" {
		t.Errorf("text path = %q", calls[1].Path)
	}
	if calls[2].Query != "timestamp=2026-01-01T00%3A00%3A00Z" {
		t.Errorf("query = %q", calls[2].Query)
	}
}

func TestUpdateRequiresAField(t *testing.T) {
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, http.StatusOK, ActionResult{})
	})

	if _, err := client.Emails.Update(context.Background(), testMailboxID, "abc", UpdateOptions{}); err == nil {
		t.Fatal("expected ErrNoUpdateFields")
	}
}

func TestClearActionSendsEmptyString(t *testing.T) {
	client, recorded := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, http.StatusOK, ActionResult{})
	})

	if _, err := client.Emails.ClearAction(context.Background(), testMailboxID, "abc"); err != nil {
		t.Fatalf("ClearAction: %v", err)
	}
	if got := (*recorded)[0].Body["action_status"]; got != "" {
		t.Errorf("action_status = %v, want empty string", got)
	}
}

func TestBlockParsesSenderEmail(t *testing.T) {
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]any{
			"action_status": "Block",
			"sender_email":  "spam@bad.test",
		})
	})

	result, err := client.Emails.Block(context.Background(), testMailboxID, "abc")
	if err != nil {
		t.Fatalf("Block: %v", err)
	}
	if result.SenderEmail != "spam@bad.test" {
		t.Errorf("sender = %q", result.SenderEmail)
	}
	if result.ActionStatus == nil || *result.ActionStatus != ActionBlock {
		t.Errorf("action = %v", result.ActionStatus)
	}
}

func TestRateLimitSnapshot(t *testing.T) {
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "3")
		w.Header().Set("X-RateLimit-Remaining", "2")
		w.Header().Set("X-RateLimit-Reset", "1767225600")
		writeEnvelope(t, w, http.StatusOK, Mailbox{ID: "m1", Email: "a@b.dev"})
	})

	if got := client.RateLimit(); got.Limit != 0 {
		t.Errorf("limit before request = %d", got.Limit)
	}
	if _, err := client.Mailboxes.Get(context.Background(), testMailboxID); err != nil {
		t.Fatalf("Get: %v", err)
	}

	snap := client.RateLimit()
	if snap.Limit != 3 || snap.Remaining != 2 {
		t.Errorf("snapshot = %+v", snap)
	}
	if !snap.Reset.Equal(time.Unix(1767225600, 0).UTC()) {
		t.Errorf("reset = %v", snap.Reset)
	}
}

func TestScanAttachmentsDecodesList(t *testing.T) {
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, http.StatusOK, []Scan{
			{AttachmentID: "a1", Filename: "f.pdf", ScanStatus: ScanProcessing},
		})
	})

	scans, err := client.Emails.ScanAttachments(context.Background(), testMailboxID, "e1")
	if err != nil {
		t.Fatalf("ScanAttachments: %v", err)
	}
	if len(scans) != 1 || scans[0].ScanStatus != ScanProcessing {
		t.Errorf("scans = %+v", scans)
	}
}

func TestContextCancellationStopsRequest(t *testing.T) {
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		writeEnvelope(t, w, http.StatusOK, Mailbox{ID: "m1", Email: "a@b.dev"})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, err := client.Mailboxes.Get(ctx, testMailboxID); err == nil {
		t.Fatal("expected context deadline error")
	}
}
