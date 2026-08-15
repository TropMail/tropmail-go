package tropmail

import (
	"context"
	"sync"
	"time"
)

// tokenBucket paces requests to the server's advertised per-second budget.
//
// It stays inert until the first X-RateLimit-Limit header is observed, so the
// very first request is never delayed. Once the tier budget is known, callers
// are spaced just enough to stay inside the server's 1-second sliding window.
type tokenBucket struct {
	mu      sync.Mutex
	rate    float64
	tokens  float64
	updated time.Time
}

func newTokenBucket() *tokenBucket {
	return &tokenBucket{updated: time.Now()}
}

// observeLimit sizes the bucket from a server-advertised per-second limit.
func (b *tokenBucket) observeLimit(limit int) {
	if limit <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.rate == float64(limit) {
		return
	}
	b.rate = float64(limit)
	b.tokens = float64(limit)
	b.updated = time.Now()
}

// reserve consumes a token and reports how long the caller must wait first.
func (b *tokenBucket) reserve() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.rate <= 0 {
		return 0
	}

	now := time.Now()
	b.tokens += now.Sub(b.updated).Seconds() * b.rate
	if b.tokens > b.rate {
		b.tokens = b.rate
	}
	b.updated = now

	if b.tokens >= 1 {
		b.tokens--
		return 0
	}
	deficit := 1 - b.tokens
	b.tokens = 0
	return time.Duration(deficit / b.rate * float64(time.Second))
}

func (b *tokenBucket) acquire(ctx context.Context) error {
	delay := b.reserve()
	if delay <= 0 {
		return nil
	}
	return sleepCtx(ctx, delay)
}
