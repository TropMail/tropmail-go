package tropmail

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
)

func emailPage(count int, total int) EmailList {
	emails := make([]Email, count)
	for i := range emails {
		emails[i] = Email{ID: "id", Timestamp: "2026-01-01T00:00:00Z"}
	}
	return EmailList{Emails: emails, Total: total, Limit: 10, Page: 1}
}

func TestAllStopsOnShortPage(t *testing.T) {
	var calls int32
	client, recorded := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			writeEnvelope(t, w, http.StatusOK, emailPage(10, 999))
			return
		}
		writeEnvelope(t, w, http.StatusOK, emailPage(3, 999))
	})

	count := 0
	for _, err := range client.Emails.All(context.Background(), ListOptions{MailboxID: testMailboxID, Limit: 10}) {
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		count++
	}

	if count != 23 {
		t.Errorf("yielded %d emails, want 23", count)
	}
	if len(*recorded) != 3 {
		t.Errorf("made %d requests, want 3", len(*recorded))
	}
}

func TestAllStopsOnEmptyPage(t *testing.T) {
	client, recorded := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, http.StatusOK, emailPage(0, 42))
	})

	for _, err := range client.Emails.All(context.Background(), ListOptions{MailboxID: testMailboxID, Limit: 10}) {
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		t.Fatal("expected no emails")
	}
	if len(*recorded) != 1 {
		t.Errorf("made %d requests, want 1", len(*recorded))
	}
}

func TestAllYieldsErrorAndStops(t *testing.T) {
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusForbidden, "Basic tier has no API access")
	})

	iterations := 0
	var seen error
	for _, err := range client.Emails.All(context.Background(), ListOptions{MailboxID: testMailboxID}) {
		iterations++
		seen = err
	}

	if iterations != 1 {
		t.Errorf("iterated %d times, want 1", iterations)
	}
	if !IsTier(seen) {
		t.Errorf("expected a tier error, got %v", seen)
	}
}

func TestAllStopsWhenCallerBreaks(t *testing.T) {
	client, recorded := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, http.StatusOK, emailPage(10, 999))
	})

	count := 0
	for range client.Emails.All(context.Background(), ListOptions{MailboxID: testMailboxID, Limit: 10}) {
		count++
		if count == 3 {
			break
		}
	}

	if count != 3 {
		t.Errorf("yielded %d, want 3", count)
	}
	if len(*recorded) != 1 {
		t.Errorf("made %d requests after break, want 1", len(*recorded))
	}
}

func TestSearchAllPagesDespiteZeroTotal(t *testing.T) {
	var calls int32
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			writeEnvelope(t, w, http.StatusOK, emailPage(5, 0))
			return
		}
		writeEnvelope(t, w, http.StatusOK, emailPage(2, 0))
	})

	count := 0
	for _, err := range client.Emails.SearchAll(context.Background(), "q", ListOptions{MailboxID: testMailboxID, Limit: 5}) {
		if err != nil {
			t.Fatalf("SearchAll: %v", err)
		}
		count++
	}
	if count != 7 {
		t.Errorf("yielded %d, want 7", count)
	}
}

func TestRetriesThenSucceeds(t *testing.T) {
	var calls int32
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.Header().Set("Retry-After", "0")
			writeError(w, http.StatusTooManyRequests, "Rate limit exceeded")
			return
		}
		writeEnvelope(t, w, http.StatusOK, Mailbox{ID: "m1", Email: "a@b.dev"})
	}, WithMaxRetries(3))

	mailbox, err := client.Mailboxes.Get(context.Background(), testMailboxID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if mailbox.ID != "m1" || atomic.LoadInt32(&calls) != 3 {
		t.Errorf("calls = %d, mailbox = %+v", calls, mailbox)
	}
}

func TestGivesUpAfterRetryBudget(t *testing.T) {
	var calls int32
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "0")
		writeError(w, http.StatusTooManyRequests, "Rate limit exceeded")
	}, WithMaxRetries(2))

	if _, err := client.Mailboxes.Get(context.Background(), testMailboxID); !IsRateLimit(err) {
		t.Fatalf("expected a rate limit error, got %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestMutationsAreNotRetried(t *testing.T) {
	var calls int32
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		writeError(w, http.StatusServiceUnavailable, "Database unavailable")
	}, WithMaxRetries(3))

	if _, err := client.Emails.Favorite(context.Background(), testMailboxID, "abc"); err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (mutations must not retry)", calls)
	}
}

func TestReadOnlyPostsAreRetried(t *testing.T) {
	var calls int32
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			writeError(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		writeEnvelope(t, w, http.StatusOK, emailPage(0, 0))
	}, WithMaxRetries(2))

	if _, err := client.Emails.List(context.Background(), ListOptions{MailboxID: testMailboxID}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestMarkdownRetriesGatewayTimeout(t *testing.T) {
	var calls int32
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			writeError(w, http.StatusGatewayTimeout, "Markdown conversion timed out")
			return
		}
		writeEnvelope(t, w, http.StatusOK, EmailDetail{ID: "abc", Content: "# Heading"})
	}, WithMaxRetries(3))

	detail, err := client.Emails.GetMarkdown(context.Background(), testMailboxID, "abc", "")
	if err != nil {
		t.Fatalf("GetMarkdown: %v", err)
	}
	if detail.Content != "# Heading" || calls != 3 {
		t.Errorf("calls = %d, content = %q", calls, detail.Content)
	}
}

func TestNonRetryableStatusIsNotRetried(t *testing.T) {
	var calls int32
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		writeError(w, http.StatusNotFound, "Email not found")
	}, WithMaxRetries(3))

	if _, err := client.Emails.Get(context.Background(), testMailboxID, "abc", GetOptions{}); !IsNotFound(err) {
		t.Fatalf("expected 404, got %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}
