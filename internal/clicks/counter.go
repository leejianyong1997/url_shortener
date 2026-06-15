package clicks

import (
	"context"
	"log"
	"sync"
	"time"
)

// Flusher persists a batch of accumulated click counts. Defined here (the
// consumer) so the Counter doesn't depend on the storage package.
type Flusher interface {
	AddClicks(ctx context.Context, counts map[string]int64) error
}

// Counter buffers click counts in memory and flushes them in batches, so the
// hot redirect path never waits on the database.
//
// It is safe for concurrent use: many request goroutines call Incr at once,
// while a background goroutine calls Flush. A single mutex guards the buffer.
type Counter struct {
	mu      sync.Mutex
	pending map[string]int64
	flusher Flusher
}

// NewCounter returns a Counter that flushes through flusher.
func NewCounter(flusher Flusher) *Counter {
	return &Counter{pending: make(map[string]int64), flusher: flusher}
}

// Incr records one visit for code. Cheap and lock-protected: take the lock,
// bump the count, release. No I/O happens here.
func (c *Counter) Incr(code string) {
	c.mu.Lock()
	c.pending[code]++
	c.mu.Unlock()
}

// Flush hands the accumulated counts to the flusher and resets the buffer.
//
// Concurrency detail that matters: we hold the lock ONLY long enough to grab
// the current batch and swap in a fresh map. The slow database write then runs
// WITHOUT the lock held, so Incr callers are never blocked on DB I/O. If the
// flush fails, the batch is merged back so no clicks are lost.
func (c *Counter) Flush(ctx context.Context) error {
	c.mu.Lock()
	if len(c.pending) == 0 {
		c.mu.Unlock()
		return nil
	}
	batch := c.pending
	c.pending = make(map[string]int64)
	c.mu.Unlock()

	if err := c.flusher.AddClicks(ctx, batch); err != nil {
		c.mu.Lock()
		for code, n := range batch {
			c.pending[code] += n
		}
		c.mu.Unlock()
		return err
	}
	return nil
}

// Run flushes buffered counts every interval until ctx is cancelled. It is
// meant to run in its own goroutine: `go counter.Run(ctx, 2*time.Second)`.
// After Run returns, the caller should Flush once more to persist anything
// buffered since the last tick (see main's graceful shutdown).
func (c *Counter) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if err := c.Flush(ctx); err != nil {
				log.Printf("clicks: periodic flush failed: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
