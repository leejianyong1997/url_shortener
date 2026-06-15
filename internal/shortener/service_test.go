package shortener

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leejianyong1997/url_shortener/internal/model"
)

// fakeStore is an in-memory Store for tests. It lets us deterministically
// simulate collisions and "not found" without touching a real database — the
// reason Service depends on the Store INTERFACE, not a concrete MySQL type.
type fakeStore struct {
	links     map[string]*model.Link // the fake "table", keyed by code
	failTimes int                    // CreateLink returns ErrCodeExists this many times first
	failWith  error                  // if set, CreateLink always returns this
	calls     int                    // CreateLink call count
	saved     *model.Link            // last link stored
}

func (f *fakeStore) CreateLink(ctx context.Context, link *model.Link) error {
	f.calls++
	if f.failWith != nil {
		return f.failWith
	}
	if f.calls <= f.failTimes {
		return ErrCodeExists
	}
	if f.links == nil {
		f.links = map[string]*model.Link{}
	}
	f.links[link.Code] = link
	f.saved = link
	return nil
}

func (f *fakeStore) FindByCode(ctx context.Context, code string) (*model.Link, error) {
	link, ok := f.links[code]
	if !ok {
		return nil, ErrNotFound
	}
	return link, nil
}

// fakeRecorder stands in for clicks.Counter in tests.
type fakeRecorder struct {
	calls    int
	lastCode string
	pending  map[string]int64
}

func (r *fakeRecorder) Incr(code string) {
	r.calls++
	r.lastCode = code
}

func (r *fakeRecorder) Pending(code string) int64 {
	return r.pending[code]
}

func TestShortenStoresLinkWithGeneratedCode(t *testing.T) {
	svc := NewService(&fakeStore{}, &fakeRecorder{})

	link, err := svc.Shorten(context.Background(), CreateParams{LongURL: "https://example.com/page"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link.LongURL != "https://example.com/page" {
		t.Errorf("got LongURL %q, want the original URL", link.LongURL)
	}
	if len(link.Code) != 7 {
		t.Errorf("got code length %d, want 7", len(link.Code))
	}
}

func TestShortenRetriesOnCollision(t *testing.T) {
	store := &fakeStore{failTimes: 2} // first two inserts "collide"
	svc := NewService(store, &fakeRecorder{})

	link, err := svc.Shorten(context.Background(), CreateParams{LongURL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.calls != 3 {
		t.Errorf("got %d attempts, want 3 (2 collisions + 1 success)", store.calls)
	}
	if link.Code == "" {
		t.Error("expected a non-empty code after retrying")
	}
}

func TestShortenGivesUpAfterMaxRetries(t *testing.T) {
	store := &fakeStore{failTimes: 1000} // every insert collides
	svc := NewService(store, &fakeRecorder{})

	_, err := svc.Shorten(context.Background(), CreateParams{LongURL: "https://example.com"})
	if err == nil {
		t.Fatal("expected an error after exhausting retries, got nil")
	}
}

func TestShortenDoesNotRetryOnOtherErrors(t *testing.T) {
	dbDown := errors.New("connection refused")
	store := &fakeStore{failWith: dbDown}
	svc := NewService(store, &fakeRecorder{})

	_, err := svc.Shorten(context.Background(), CreateParams{LongURL: "https://example.com"})
	if !errors.Is(err, dbDown) {
		t.Errorf("expected the underlying error to be preserved, got %v", err)
	}
	if store.calls != 1 {
		t.Errorf("a non-collision error must NOT be retried; got %d calls", store.calls)
	}
}

func TestResolveReturnsLinkAndCountsClick(t *testing.T) {
	store := &fakeStore{links: map[string]*model.Link{
		"abc1234": {Code: "abc1234", LongURL: "https://example.com"},
	}}
	recorder := &fakeRecorder{}
	svc := NewService(store, recorder)

	link, err := svc.Resolve(context.Background(), "abc1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link.LongURL != "https://example.com" {
		t.Errorf("got %q, want the stored URL", link.LongURL)
	}
	if recorder.calls != 1 || recorder.lastCode != "abc1234" {
		t.Errorf("expected the click to be recorded once for abc1234, got %d call(s) for %q",
			recorder.calls, recorder.lastCode)
	}
}

func TestResolveReturnsNotFoundForMissingCode(t *testing.T) {
	recorder := &fakeRecorder{}
	svc := NewService(&fakeStore{}, recorder)

	_, err := svc.Resolve(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if recorder.calls != 0 {
		t.Error("a missing code must NOT be counted as a click")
	}
}

func TestShortenUsesCustomAlias(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, &fakeRecorder{})

	link, err := svc.Shorten(context.Background(), CreateParams{LongURL: "https://example.com", Alias: "my-link"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link.Code != "my-link" {
		t.Errorf("got code %q, want the custom alias my-link", link.Code)
	}
	if store.calls != 1 {
		t.Errorf("a custom alias must be a single attempt; got %d calls", store.calls)
	}
}

func TestShortenReturnsConflictForTakenAlias(t *testing.T) {
	store := &fakeStore{failTimes: 1} // the alias is already taken
	svc := NewService(store, &fakeRecorder{})

	_, err := svc.Shorten(context.Background(), CreateParams{LongURL: "https://example.com", Alias: "taken"})
	if !errors.Is(err, ErrCodeExists) {
		t.Errorf("expected ErrCodeExists for a taken alias, got %v", err)
	}
	if store.calls != 1 {
		t.Errorf("a taken alias must NOT be retried; got %d calls", store.calls)
	}
}

func TestStatsAddsBufferedClicksToStoredValue(t *testing.T) {
	store := &fakeStore{links: map[string]*model.Link{
		"abc1234": {Code: "abc1234", LongURL: "https://example.com", Clicks: 5},
	}}
	recorder := &fakeRecorder{pending: map[string]int64{"abc1234": 3}}
	svc := NewService(store, recorder)

	link, err := svc.Stats(context.Background(), "abc1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link.Clicks != 8 {
		t.Errorf("Clicks = %d, want 8 (5 stored + 3 buffered)", link.Clicks)
	}
}

func TestStatsReturnsNotFoundForMissingCode(t *testing.T) {
	svc := NewService(&fakeStore{}, &fakeRecorder{})

	_, err := svc.Stats(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestShortenSetsExpiry(t *testing.T) {
	svc := NewService(&fakeStore{}, &fakeRecorder{})
	exp := time.Now().Add(time.Hour)

	link, err := svc.Shorten(context.Background(), CreateParams{LongURL: "https://example.com", ExpiresAt: &exp})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link.ExpiresAt == nil || !link.ExpiresAt.Equal(exp) {
		t.Errorf("ExpiresAt = %v, want %v", link.ExpiresAt, exp)
	}
}

func TestResolveReturnsGoneForExpiredLink(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	store := &fakeStore{links: map[string]*model.Link{
		"old": {Code: "old", LongURL: "https://example.com", ExpiresAt: &past},
	}}
	recorder := &fakeRecorder{}
	svc := NewService(store, recorder)

	_, err := svc.Resolve(context.Background(), "old")
	if !errors.Is(err, ErrGone) {
		t.Errorf("expected ErrGone for an expired link, got %v", err)
	}
	if recorder.calls != 0 {
		t.Error("an expired link must NOT be counted as a click")
	}
}
