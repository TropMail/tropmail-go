package tropmail

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status  int
		check   func(error) bool
		checkID string
	}{
		{http.StatusBadRequest, IsValidation, "IsValidation"},
		{http.StatusUnauthorized, IsAuth, "IsAuth"},
		{http.StatusForbidden, IsTier, "IsTier"},
		{http.StatusNotFound, IsNotFound, "IsNotFound"},
		{http.StatusTooManyRequests, IsRateLimit, "IsRateLimit"},
		{http.StatusGatewayTimeout, IsMarkdownTimeout, "IsMarkdownTimeout"},
		{http.StatusInternalServerError, IsServer, "IsServer"},
	}

	for _, tc := range cases {
		t.Run(tc.checkID, func(t *testing.T) {
			client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeError(w, tc.status, "boom")
			})

			_, err := client.Mailbox.Get(context.Background())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !tc.check(err) {
				t.Errorf("%s returned false for HTTP %d: %v", tc.checkID, tc.status, err)
			}

			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("errors.As failed for %v", err)
			}
			if apiErr.Status != tc.status {
				t.Errorf("status = %d, want %d", apiErr.Status, tc.status)
			}
			if apiErr.Message != "boom" {
				t.Errorf("message = %q", apiErr.Message)
			}
			if apiErr.RequestID != "req-err" {
				t.Errorf("request id = %q", apiErr.RequestID)
			}
		})
	}
}

func TestPlainTextNotFoundIsStillTyped(t *testing.T) {
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	})

	_, err := client.Mailbox.Get(context.Background())
	if !IsNotFound(err) {
		t.Fatalf("expected a 404, got %v", err)
	}

	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Message != "Not Found" {
		t.Errorf("message = %v", err)
	}
}

func TestRetryAfterIsCarried(t *testing.T) {
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "2")
		writeError(w, http.StatusTooManyRequests, "Rate limit exceeded")
	})

	_, err := client.Mailbox.Get(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As failed: %v", err)
	}
	if apiErr.RetryAfter.Seconds() != 2 {
		t.Errorf("retry after = %v", apiErr.RetryAfter)
	}
}

func TestSuccessFalseOn200IsAnError(t *testing.T) {
	client, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"message":"nope","data":null,"error":"nope"}`))
	})

	_, err := client.Mailbox.Get(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Message != "nope" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTransportFailureIsConnectionError(t *testing.T) {
	client, err := New(
		testAPIKey,
		WithBaseURL("http://127.0.0.1:1/api/v1"),
		WithMaxRetries(0),
		WithThrottle(false),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.Mailbox.Get(context.Background())
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if !IsConnection(err) {
		t.Errorf("IsConnection returned false for %v", err)
	}
}
