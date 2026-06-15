package clicks

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeFlusher records the counts it receives. failOn lets a test make the Nth
// flush fail, so we can check counts are not lost.
type fakeFlusher struct {
	totals map[string]int64
	calls  int
	failOn int
}

func newFakeFlusher() *fakeFlusher { return &fakeFlusher{totals: map[string]int64{}} }

func (f *fakeFlusher) AddClicks(ctx context.Context, counts map[string]int64) error {
	f.calls++
	if f.failOn == f.calls {
		return errors.New("flush failed")
	}
	for code, n := range counts {
		f.totals[code] += n
	}
	return nil
}

func TestCounterAggregatesBeforeFlush(t *testing.T) {
	f := newFakeFlusher()
	c := NewCounter(f)

	c.Incr("abc")
	c.Incr("abc")
	c.Incr("xyz")

	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.totals["abc"] != 2 || f.totals["xyz"] != 1 {
		t.Errorf("got %v, want abc=2 xyz=1", f.totals)
	}
}

// TestCounterConcurrentIncrementsAreNotLost is the race demo: many goroutines
// hammer the SAME code at once. The final total must be exact. Run with
// `go test -race` to prove there is no data race.
func TestCounterConcurrentIncrementsAreNotLost(t *testing.T) {
	f := newFakeFlusher()
	c := NewCounter(f)

	const goroutines = 100
	const perGoroutine = 100

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				c.Incr("hot")
			}
		}()
	}
	wg.Wait()

	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := int64(goroutines * perGoroutine) // 10_000
	if f.totals["hot"] != want {
		t.Errorf("got %d, want %d — lost updates under concurrency", f.totals["hot"], want)
	}
}

func TestFlushOnEmptyBufferIsNoOp(t *testing.T) {
	f := newFakeFlusher()
	c := NewCounter(f)

	c.Incr("abc")
	_ = c.Flush(context.Background()) // sends {abc:1}
	_ = c.Flush(context.Background()) // nothing pending -> must not call flusher

	if f.calls != 1 {
		t.Errorf("second flush should be a no-op; got %d flusher calls", f.calls)
	}
}

func TestFlushKeepsCountsWhenFlusherFails(t *testing.T) {
	f := &fakeFlusher{totals: map[string]int64{}, failOn: 1} // first flush fails
	c := NewCounter(f)

	c.Incr("abc")
	c.Incr("abc")

	if err := c.Flush(context.Background()); err == nil {
		t.Fatal("expected the first flush to fail")
	}
	if err := c.Flush(context.Background()); err != nil { // retry succeeds
		t.Fatalf("unexpected error on retry: %v", err)
	}
	if f.totals["abc"] != 2 {
		t.Errorf("counts were lost after a failed flush: got %d, want 2", f.totals["abc"])
	}
}

func TestPendingReflectsUnflushedCount(t *testing.T) {
	c := NewCounter(newFakeFlusher())

	c.Incr("abc")
	c.Incr("abc")
	c.Incr("xyz")

	if got := c.Pending("abc"); got != 2 {
		t.Errorf("Pending(abc) = %d, want 2", got)
	}
	if got := c.Pending("unknown"); got != 0 {
		t.Errorf("Pending(unknown) = %d, want 0", got)
	}

	_ = c.Flush(context.Background())
	if got := c.Pending("abc"); got != 0 {
		t.Errorf("after flush, Pending(abc) = %d, want 0", got)
	}
}
