package tropmail

import (
	"net/http"
	"strconv"
	"time"
)

// RateLimitSnapshot holds the rate-limit window from the most recent response.
type RateLimitSnapshot struct {
	Limit     int
	Remaining int
	Reset     time.Time
	UpdatedAt time.Time
}

// RateLimit returns the latest observed rate-limit window.
func (c *Client) RateLimit() RateLimitSnapshot {
	c.rateLimitMu.RLock()
	defer c.rateLimitMu.RUnlock()
	return c.rateLimit
}

func (c *Client) updateRateLimit(header http.Header) {
	limitStr := header.Get("X-RateLimit-Limit")
	remainingStr := header.Get("X-RateLimit-Remaining")
	resetStr := header.Get("X-RateLimit-Reset")
	if limitStr == "" && remainingStr == "" && resetStr == "" {
		return
	}

	snap := RateLimitSnapshot{UpdatedAt: time.Now().UTC()}
	if v, err := strconv.Atoi(limitStr); err == nil {
		snap.Limit = v
	}
	if v, err := strconv.Atoi(remainingStr); err == nil {
		snap.Remaining = v
	}
	if v, err := strconv.ParseInt(resetStr, 10, 64); err == nil && v > 0 {
		snap.Reset = time.Unix(v, 0).UTC()
	}

	c.rateLimitMu.Lock()
	c.rateLimit = snap
	c.rateLimitMu.Unlock()

	if c.bucket != nil && snap.Limit > 0 {
		c.bucket.observeLimit(snap.Limit)
	}
}
