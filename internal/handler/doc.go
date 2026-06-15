// Package handler is the HTTP layer: each handler decodes the request, calls
// the business logic in package shortener, and encodes the response. It is the
// only layer that touches http.ResponseWriter / *http.Request.
// (≈ Controllers in Laravel.)
package handler
