package tropmail

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Error is the structured error returned by the SDK for API and transport failures.
type Error struct {
	Status     int
	Message    string
	RequestID  string
	RetryAfter time.Duration
	Body       string
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message != "" {
		if e.Status > 0 {
			return fmt.Sprintf("tropmail: %s (HTTP %d)", e.Message, e.Status)
		}
		return fmt.Sprintf("tropmail: %s", e.Message)
	}
	if e.Status > 0 {
		return fmt.Sprintf("tropmail: HTTP %d", e.Status)
	}
	return "tropmail: request failed"
}

// IsAuth reports whether err is a 401 authentication failure.
func IsAuth(err error) bool {
	return hasStatus(err, 401)
}

// IsNotFound reports whether err is a 404 not-found failure.
func IsNotFound(err error) bool {
	return hasStatus(err, 404)
}

// IsRateLimit reports whether err is a 429 rate-limit failure.
func IsRateLimit(err error) bool {
	return hasStatus(err, 429)
}

// IsValidation reports whether err is a 400 validation failure.
func IsValidation(err error) bool {
	return hasStatus(err, 400)
}

// IsTier reports whether err is a 403 tier/access restriction.
func IsTier(err error) bool {
	return hasStatus(err, 403)
}

// IsServer reports whether err is a 5xx server failure (excluding markdown timeout).
func IsServer(err error) bool {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Status >= 500 && apiErr.Status != 504
}

// IsMarkdownTimeout reports whether err is a 504 markdown conversion timeout.
func IsMarkdownTimeout(err error) bool {
	return hasStatus(err, 504)
}

// IsConnection reports whether err is a transport/network failure.
func IsConnection(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "tls:")
}

func hasStatus(err error, status int) bool {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.Status == status
	}
	return false
}

func newAPIError(status int, message, requestID, body string, retryAfter time.Duration) *Error {
	return &Error{
		Status:     status,
		Message:    message,
		RequestID:  requestID,
		RetryAfter: retryAfter,
		Body:       body,
	}
}

func wrapTransportError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return err
	}
	return fmt.Errorf("tropmail: %w", err)
}
