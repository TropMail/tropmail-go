// Package tropmail is the official Go client for the TropMail API.
//
// Create a client with [New] and reach the API through its services:
//
//	client, err := tropmail.New(os.Getenv("TROPMAIL_API_KEY"))
//	if err != nil {
//		return err
//	}
//	listed, err := client.Mailboxes.List(ctx)
//
// Every call takes a [context.Context]. Reads are retried with jittered backoff;
// mutations are not.
package tropmail

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the production API endpoint.
const DefaultBaseURL = "https://api.tropmail.com/api/v1"

const (
	// DefaultTimeout is generous because the markdown view blocks ~60s server-side.
	DefaultTimeout = 120 * time.Second
	// DefaultMaxRetries bounds automatic retries of idempotent requests.
	DefaultMaxRetries = 3

	maxResponseBytes = 8 << 20
	userAgent        = "tropmail-go/" + Version
)

// Version is the SDK version reported in the User-Agent header.
const Version = "1.1.0"

var retryableStatuses = map[int]bool{
	http.StatusTooManyRequests:    true,
	http.StatusBadGateway:         true,
	http.StatusServiceUnavailable: true,
	http.StatusGatewayTimeout:     true,
}

// Client is a TropMail API client. It is safe for concurrent use.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	maxRetries int
	bucket     *tokenBucket
	userAgent  string

	rateLimitMu sync.RWMutex
	rateLimit   RateLimitSnapshot

	// Mailboxes covers GET /mailboxes and GET /mailboxes/{id}.
	Mailboxes *MailboxesService
	// Emails covers /mailboxes/{id}/emails.
	Emails *EmailsService
	// Attachments covers /mailboxes/{id}/attachments/{attId}.
	Attachments *AttachmentsService
}

// Option configures a [Client].
type Option func(*Client)

// WithBaseURL overrides the API base URL, including the /api/v1 suffix.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

// WithHTTPClient supplies a custom [http.Client], replacing the tuned default.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithTimeout sets the per-request timeout on the default HTTP client.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.httpClient.Timeout = timeout
		}
	}
}

// WithMaxRetries bounds retries of idempotent requests. Zero disables them.
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		if n >= 0 {
			c.maxRetries = n
		}
	}
}

// WithThrottle enables or disables client-side pacing to the tier budget.
// Pacing is on by default and keeps well-behaved callers off the 429 path.
func WithThrottle(enabled bool) Option {
	return func(c *Client) {
		if enabled {
			if c.bucket == nil {
				c.bucket = newTokenBucket()
			}
			return
		}
		c.bucket = nil
	}
}

// WithUserAgent appends a caller identifier to the SDK User-Agent.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua + " " + userAgent
		}
	}
}

// newTransport returns a transport tuned for a small number of hot connections
// to a single API host.
func newTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}

// New creates a client. The key falls back to TROPMAIL_API_KEY when empty and is
// format-checked locally so a malformed key fails before any network call.
func New(apiKey string, opts ...Option) (*Client, error) {
	key, err := APIKeyFromEnv(apiKey)
	if err != nil {
		return nil, err
	}

	c := &Client{
		baseURL:    DefaultBaseURL,
		apiKey:     key,
		httpClient: &http.Client{Timeout: DefaultTimeout, Transport: newTransport()},
		maxRetries: DefaultMaxRetries,
		bucket:     newTokenBucket(),
		userAgent:  userAgent,
	}
	for _, opt := range opts {
		opt(c)
	}

	c.Mailboxes = &MailboxesService{client: c}
	c.Emails = &EmailsService{client: c}
	c.Attachments = &AttachmentsService{client: c}
	return c, nil
}

// BaseURL returns the configured API base URL.
func (c *Client) BaseURL() string { return c.baseURL }

// HTTPClient exposes the underlying HTTP client, for example to fetch a
// download URL with the same connection pool.
func (c *Client) HTTPClient() *http.Client { return c.httpClient }

// request describes a single API call.
type request struct {
	method  string
	path    string
	body    any
	query   url.Values
	noAuth  bool
	noRetry bool
}

type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Error   any             `json:"error"`
}

func (r request) retryable() bool {
	if r.noRetry {
		return false
	}
	if r.method == http.MethodGet {
		return true
	}
	return false
}

func fullJitter(attempt int) time.Duration {
	const base = 500 * time.Millisecond
	const cap = 30 * time.Second
	ceiling := base << attempt
	if ceiling > cap {
		ceiling = cap
	}
	return time.Duration(rand.Int63n(int64(ceiling) + 1))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// do performs a request, unwraps the envelope, and decodes data into out.
func (c *Client) do(ctx context.Context, r request, out any) error {
	var payload []byte
	if r.body != nil {
		encoded, err := json.Marshal(r.body)
		if err != nil {
			return fmt.Errorf("tropmail: encode request: %w", err)
		}
		payload = encoded
	}

	endpoint := c.baseURL + r.path
	if len(r.query) > 0 {
		endpoint += "?" + r.query.Encode()
	}

	attempts := 1
	if r.retryable() {
		attempts = c.maxRetries + 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if c.bucket != nil {
			if err := c.bucket.acquire(ctx); err != nil {
				return err
			}
		}

		status, body, header, err := c.roundTrip(ctx, r, endpoint, payload)
		if err != nil {
			lastErr = err
			if attempt >= attempts-1 || ctx.Err() != nil {
				return wrapTransportError(err)
			}
			if sleepErr := sleepCtx(ctx, fullJitter(attempt)); sleepErr != nil {
				return sleepErr
			}
			continue
		}

		c.updateRateLimit(header)

		if status >= 400 {
			apiErr := c.parseError(status, body, header)
			if attempt < attempts-1 && retryableStatuses[status] {
				delay := fullJitter(attempt)
				if status == http.StatusTooManyRequests && apiErr.RetryAfter > 0 {
					delay = apiErr.RetryAfter
				}
				if sleepErr := sleepCtx(ctx, delay); sleepErr != nil {
					return sleepErr
				}
				lastErr = apiErr
				continue
			}
			return apiErr
		}

		return decodeEnvelope(status, body, header, out)
	}

	if lastErr != nil {
		return wrapTransportError(lastErr)
	}
	return fmt.Errorf("tropmail: request failed after %d attempts", attempts)
}

func (c *Client) roundTrip(
	ctx context.Context,
	r request,
	endpoint string,
	payload []byte,
) (int, []byte, http.Header, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, r.method, endpoint, reader)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-Request-ID", newRequestID())
	if !r.noAuth {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("read response: %w", err)
	}
	return resp.StatusCode, body, resp.Header, nil
}

// parseError builds a typed error, tolerating the plain-text body that unknown
// protected routes return instead of the JSON envelope.
func (c *Client) parseError(status int, body []byte, header http.Header) *Error {
	requestID := header.Get("X-Request-ID")
	retryAfter := parseRetryAfter(header)

	var env envelope
	message := ""
	if err := json.Unmarshal(body, &env); err == nil {
		message = env.Message
		if message == "" {
			if s, ok := env.Error.(string); ok {
				message = s
			}
		}
	}
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return newAPIError(status, message, requestID, string(body), retryAfter)
}

func decodeEnvelope(status int, body []byte, header http.Header, out any) error {
	requestID := header.Get("X-Request-ID")

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return newAPIError(
			status,
			fmt.Sprintf("decode response: %v", err),
			requestID,
			string(body),
			0,
		)
	}

	if !env.Success {
		message := env.Message
		if message == "" {
			if s, ok := env.Error.(string); ok {
				message = s
			}
		}
		if message == "" {
			message = "request failed"
		}
		return newAPIError(status, message, requestID, string(body), 0)
	}

	if out == nil {
		return nil
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return newAPIError(status, "response data is null", requestID, string(body), 0)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return newAPIError(
			status,
			fmt.Sprintf("decode data: %v", err),
			requestID,
			string(body),
			0,
		)
	}
	return nil
}

// openBinary performs an authenticated GET that returns a raw body stream
// (attachment download). Caller must Close the reader.
func (c *Client) openBinary(ctx context.Context, r request) (io.ReadCloser, error) {
	endpoint := c.baseURL + r.path
	if len(r.query) > 0 {
		endpoint += "?" + r.query.Encode()
	}

	if c.bucket != nil {
		if err := c.bucket.acquire(ctx); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, r.method, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("tropmail: create request: %w", err)
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-Request-ID", newRequestID())
	if !r.noAuth {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, wrapTransportError(err)
	}
	c.updateRateLimit(resp.Header)

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		return nil, c.parseError(resp.StatusCode, body, resp.Header)
	}
	return resp.Body, nil
}

func parseRetryAfter(header http.Header) time.Duration {
	value := header.Get("Retry-After")
	if value == "" {
		return 0
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func newRequestID() string {
	var buf [16]byte
	if _, err := crand.Read(buf[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}
