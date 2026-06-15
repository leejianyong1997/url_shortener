package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/leejianyong1997/url_shortener/internal/model"
	"github.com/leejianyong1997/url_shortener/internal/shortener"
)

// fakeShortener stands in for *shortener.Service so handlers can be tested with
// no real service or database. Each test sets only the func it needs.
type fakeShortener struct {
	shortenFn func(ctx context.Context, longURL string) (*model.Link, error)
	resolveFn func(ctx context.Context, code string) (*model.Link, error)
	statsFn   func(ctx context.Context, code string) (*model.Link, error)
}

func (f *fakeShortener) Shorten(ctx context.Context, longURL string) (*model.Link, error) {
	return f.shortenFn(ctx, longURL)
}

func (f *fakeShortener) Resolve(ctx context.Context, code string) (*model.Link, error) {
	return f.resolveFn(ctx, code)
}

func (f *fakeShortener) Stats(ctx context.Context, code string) (*model.Link, error) {
	return f.statsFn(ctx, code)
}

func TestShortenReturns201WithShortURL(t *testing.T) {
	fake := &fakeShortener{
		shortenFn: func(ctx context.Context, longURL string) (*model.Link, error) {
			return &model.Link{Code: "abc1234", LongURL: longURL}, nil
		},
	}
	h := New(fake, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(`{"url":"https://example.com"}`))
	rec := httptest.NewRecorder()

	h.Shorten(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("got status %d, want 201", rec.Code)
	}
	var resp struct {
		Code     string `json:"code"`
		ShortURL string `json:"short_url"`
		LongURL  string `json:"long_url"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response was not valid JSON: %v", err)
	}
	if resp.ShortURL != "http://localhost:8080/abc1234" {
		t.Errorf("got short_url %q, want http://localhost:8080/abc1234", resp.ShortURL)
	}
}

func TestShortenRejectsInvalidURL(t *testing.T) {
	fake := &fakeShortener{
		shortenFn: func(ctx context.Context, longURL string) (*model.Link, error) {
			t.Fatal("service must NOT be called for an invalid URL")
			return nil, nil
		},
	}
	h := New(fake, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(`{"url":"not-a-url"}`))
	rec := httptest.NewRecorder()

	h.Shorten(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", rec.Code)
	}
}

func TestShortenRejectsEmptyURL(t *testing.T) {
	fake := &fakeShortener{
		shortenFn: func(ctx context.Context, longURL string) (*model.Link, error) {
			t.Fatal("service must NOT be called for an empty URL")
			return nil, nil
		},
	}
	h := New(fake, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(`{"url":"   "}`))
	rec := httptest.NewRecorder()

	h.Shorten(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", rec.Code)
	}
}

func TestShortenRejectsMalformedJSON(t *testing.T) {
	h := New(&fakeShortener{}, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(`{bad json`))
	rec := httptest.NewRecorder()

	h.Shorten(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", rec.Code)
	}
}

func TestRedirectReturns302WithLocation(t *testing.T) {
	fake := &fakeShortener{
		resolveFn: func(ctx context.Context, code string) (*model.Link, error) {
			return &model.Link{Code: code, LongURL: "https://example.com/dest"}, nil
		},
	}
	h := New(fake, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodGet, "/abc1234", nil)
	req.SetPathValue("code", "abc1234")
	rec := httptest.NewRecorder()

	h.Redirect(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("got status %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://example.com/dest" {
		t.Errorf("got Location %q, want the original URL", loc)
	}
}

func TestRedirectReturns404ForUnknownCode(t *testing.T) {
	fake := &fakeShortener{
		resolveFn: func(ctx context.Context, code string) (*model.Link, error) {
			return nil, shortener.ErrNotFound
		},
	}
	h := New(fake, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.SetPathValue("code", "missing")
	rec := httptest.NewRecorder()

	h.Redirect(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", rec.Code)
	}
}

func TestStatsReturns200WithClickCount(t *testing.T) {
	created := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	fake := &fakeShortener{
		statsFn: func(ctx context.Context, code string) (*model.Link, error) {
			return &model.Link{Code: code, LongURL: "https://example.com", Clicks: 42, CreatedAt: created}, nil
		},
	}
	h := New(fake, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodGet, "/api/links/abc1234/stats", nil)
	req.SetPathValue("code", "abc1234")
	rec := httptest.NewRecorder()

	h.Stats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	var resp struct {
		Code     string `json:"code"`
		ShortURL string `json:"short_url"`
		Clicks   int64  `json:"clicks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response was not valid JSON: %v", err)
	}
	if resp.Clicks != 42 {
		t.Errorf("got clicks %d, want 42", resp.Clicks)
	}
	if resp.ShortURL != "http://localhost:8080/abc1234" {
		t.Errorf("got short_url %q", resp.ShortURL)
	}
}

func TestStatsReturns404ForUnknownCode(t *testing.T) {
	fake := &fakeShortener{
		statsFn: func(ctx context.Context, code string) (*model.Link, error) {
			return nil, shortener.ErrNotFound
		},
	}
	h := New(fake, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodGet, "/api/links/missing/stats", nil)
	req.SetPathValue("code", "missing")
	rec := httptest.NewRecorder()

	h.Stats(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", rec.Code)
	}
}
