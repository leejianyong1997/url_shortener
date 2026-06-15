package shortener

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leejianyong1997/url_shortener/internal/model"
)

// ErrCodeExists signals that a short code already exists (a collision). A Store
// implementation returns it so the Service knows to retry with a fresh code.
var ErrCodeExists = errors.New("short code already exists")

// ErrNotFound signals that no link exists for a given code. The redirect
// handler turns this into an HTTP 404.
var ErrNotFound = errors.New("link not found")

// ErrGone signals that a link exists but has expired. The redirect handler
// turns this into an HTTP 410 Gone.
var ErrGone = errors.New("link has expired")

// Store is the persistence contract the Service needs — nothing more.
//
// Idiomatic Go: the interface is declared HERE, in the consumer, not in the
// package that implements it. The Service depends on this small abstraction, so
// in tests we pass a fake (see service_test.go) and in production we pass the
// MySQL implementation from package storage. "Accept interfaces."
type Store interface {
	CreateLink(ctx context.Context, link *model.Link) error
	FindByCode(ctx context.Context, code string) (*model.Link, error)
}

// ClickRecorder records a visit for a code. The production implementation
// (clicks.Counter) buffers in memory and flushes in the background, so Incr is
// cheap, non-blocking, and never returns an error on the hot redirect path.
type ClickRecorder interface {
	Incr(code string)
	// Pending reports clicks buffered in memory but not yet persisted, so Stats
	// can return an exact total despite the write-behind buffer.
	Pending(code string) int64
}

// CreateParams are the inputs for creating a short link. Only LongURL is
// required; Alias and ExpiresAt are optional (a params struct keeps the call
// readable as optional inputs grow).
type CreateParams struct {
	LongURL   string
	Alias     string     // optional user-chosen code
	ExpiresAt *time.Time // optional expiry; nil = never expires
}

// Service turns long URLs into stored short links.
type Service struct {
	store      Store
	clicks     ClickRecorder
	codeLength int
	maxRetries int
}

// NewService wires a Service to its Store and ClickRecorder. This is
// constructor-style dependency injection: the caller (main) decides what to
// inject.
func NewService(store Store, clicks ClickRecorder) *Service {
	return &Service{
		store:      store,
		clicks:     clicks,
		codeLength: 7, // 62^7 ≈ 3.5e12 combinations
		maxRetries: 5, // collisions are rare; a few retries is plenty
	}
}

// Shorten stores longURL under a short code and returns the new link. If alias
// is non-empty it is used verbatim as the code, and a collision is a hard
// conflict (ErrCodeExists) rather than a retry — the caller asked for that exact
// code. If alias is empty, a random base62 code is generated.
//
// Uniqueness strategy for the random case (the interview answer):
//  1. Generate a random base62 code.
//  2. Try to insert it. The DB's UNIQUE index on `code` is the source of truth.
//  3. If the insert collides (ErrCodeExists), the code was already taken, so we
//     loop and generate a NEW random code. Each retry is independent.
//  4. Any OTHER error (e.g. DB down) is NOT a collision, so we fail immediately
//     instead of pointlessly retrying.
//  5. After maxRetries collisions we give up with an error. With a 3.5-trillion
//     keyspace this effectively never happens, but the cap prevents an infinite
//     loop if the table somehow filled up.
func (s *Service) Shorten(ctx context.Context, p CreateParams) (*model.Link, error) {
	if p.Alias != "" {
		link := &model.Link{Code: p.Alias, LongURL: p.LongURL, ExpiresAt: p.ExpiresAt}
		if err := s.store.CreateLink(ctx, link); err != nil {
			if errors.Is(err, ErrCodeExists) {
				return nil, ErrCodeExists // taken — handler turns this into 409
			}
			return nil, fmt.Errorf("create link: %w", err)
		}
		return link, nil
	}

	for attempt := 0; attempt < s.maxRetries; attempt++ {
		code, err := GenerateCode(s.codeLength)
		if err != nil {
			return nil, fmt.Errorf("generate code: %w", err)
		}

		link := &model.Link{Code: code, LongURL: p.LongURL, ExpiresAt: p.ExpiresAt}
		err = s.store.CreateLink(ctx, link)
		switch {
		case err == nil:
			return link, nil // success
		case errors.Is(err, ErrCodeExists):
			continue // collision — try a brand-new random code
		default:
			return nil, fmt.Errorf("create link: %w", err) // real failure
		}
	}
	return nil, fmt.Errorf("could not generate a unique code after %d attempts", s.maxRetries)
}

// Resolve looks up the link for code and records one visit — the read path
// behind the redirect endpoint.
//
// If no link exists, FindByCode returns ErrNotFound and we pass it straight up
// so the handler can return a 404. For simplicity a failed click-count is
// treated as a real error; a fancier version might count clicks best-effort so
// an analytics hiccup never breaks a redirect.
func (s *Service) Resolve(ctx context.Context, code string) (*model.Link, error) {
	link, err := s.store.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if link.ExpiresAt != nil && time.Now().After(*link.ExpiresAt) {
		return nil, ErrGone // exists but expired — handler returns 410
	}
	// Record the visit in memory and return immediately — the redirect never
	// waits on a database write. The background flusher persists it later.
	s.clicks.Incr(code)
	return link, nil
}

// Stats returns the link for code with an accurate click total: the value
// stored in the database PLUS any clicks still buffered in memory. Unlike
// Resolve, it does not count a visit.
func (s *Service) Stats(ctx context.Context, code string) (*model.Link, error) {
	link, err := s.store.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	link.Clicks += s.clicks.Pending(code)
	return link, nil
}
