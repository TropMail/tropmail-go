package tropmail

import "testing"

func TestBucketIsInertUntilALimitIsObserved(t *testing.T) {
	bucket := newTokenBucket()
	for i := 0; i < 100; i++ {
		if delay := bucket.reserve(); delay != 0 {
			t.Fatalf("delay = %v before any limit is known", delay)
		}
	}
}

func TestBucketSpendsBudgetThenWaits(t *testing.T) {
	bucket := newTokenBucket()
	bucket.observeLimit(3)

	for i := 0; i < 3; i++ {
		if delay := bucket.reserve(); delay != 0 {
			t.Fatalf("token %d delayed by %v", i, delay)
		}
	}
	if delay := bucket.reserve(); delay <= 0 {
		t.Error("fourth token should wait")
	}
}

func TestObservingTheSameLimitDoesNotRefill(t *testing.T) {
	bucket := newTokenBucket()
	bucket.observeLimit(2)
	bucket.reserve()
	bucket.reserve()
	bucket.observeLimit(2)

	if delay := bucket.reserve(); delay <= 0 {
		t.Error("re-observing the same limit must not refill the bucket")
	}
}

func TestTierChangeResizesBucket(t *testing.T) {
	bucket := newTokenBucket()
	bucket.observeLimit(1)
	bucket.reserve()
	bucket.observeLimit(50)

	if delay := bucket.reserve(); delay != 0 {
		t.Errorf("delay = %v after upgrade", delay)
	}
}

func TestIgnoresNonsenseLimits(t *testing.T) {
	bucket := newTokenBucket()
	bucket.observeLimit(0)
	bucket.observeLimit(-5)

	if delay := bucket.reserve(); delay != 0 {
		t.Errorf("delay = %v, bucket should still be inert", delay)
	}
}
