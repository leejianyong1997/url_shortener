package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/leejianyong1997/url_shortener/internal/model"
	"github.com/leejianyong1997/url_shortener/internal/shortener"
)

// Shortener is the business-logic contract the HTTP layer needs. Declared here,
// in the consumer, so handlers can be driven by a fake in tests.
type Shortener interface {
	Shorten(ctx context.Context, longURL string) (*model.Link, error)
	Resolve(ctx context.Context, code string) (*model.Link, error)
}

// Handler carries the dependencies every route needs.
type Handler struct {
	svc     Shortener
	baseURL string // e.g. http://localhost:8080, used to build the short URL
}

// New wires a Handler to its service.
func New(svc Shortener, baseURL string) *Handler {
	return &Handler{svc: svc, baseURL: baseURL}
}

// shortenRequest/shortenResponse are the JSON shapes for POST /shorten. The
// `json:"..."` struct tags map Go field names <-> JSON keys.
type shortenRequest struct {
	URL string `json:"url"`
}

type shortenResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
	LongURL  string `json:"long_url"`
}

// Shorten handles POST /shorten: read JSON, validate, create, return JSON.
func (h *Handler) Shorten(w http.ResponseWriter, r *http.Request) {
	var req shortenRequest
	// Decoder reads the request body stream and fills the struct by json tag.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be valid JSON")
		return
	}

	longURL := strings.TrimSpace(req.URL)
	if longURL == "" {
		writeError(w, http.StatusBadRequest, "field 'url' is required")
		return
	}
	if !isValidHTTPURL(longURL) {
		writeError(w, http.StatusBadRequest, "field 'url' must be a valid http(s) URL")
		return
	}

	link, err := h.svc.Shorten(r.Context(), longURL)
	if err != nil {
		// Log the real cause server-side; return a generic message to clients.
		log.Printf("shorten %q: %v", longURL, err)
		writeError(w, http.StatusInternalServerError, "could not create short link")
		return
	}

	writeJSON(w, http.StatusCreated, shortenResponse{
		Code:     link.Code,
		ShortURL: h.baseURL + "/" + link.Code,
		LongURL:  link.LongURL,
	})
}

// Redirect handles GET /{code}: resolve the code, count the click, 302 to it.
func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code") // populated by the "GET /{code}" route pattern

	link, err := h.svc.Resolve(r.Context(), code)
	if err != nil {
		// This is Go error handling instead of try/catch: we branch on the
		// error VALUE. A known "not found" becomes 404; anything else is 500.
		if errors.Is(err, shortener.ErrNotFound) {
			writeError(w, http.StatusNotFound, "short link not found")
			return
		}
		log.Printf("resolve %q: %v", code, err)
		writeError(w, http.StatusInternalServerError, "could not resolve short link")
		return
	}

	// 302 Found = TEMPORARY redirect. We deliberately avoid 301 (permanent),
	// because browsers cache 301s hard: they would stop hitting us and our
	// click counter would silently stop. 302 keeps every visit flowing through.
	http.Redirect(w, r, link.LongURL, http.StatusFound)
}

// isValidHTTPURL accepts only absolute http/https URLs with a host.
func isValidHTTPURL(s string) bool {
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// writeJSON encodes payload as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload) // body already started; nothing to do on error
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
