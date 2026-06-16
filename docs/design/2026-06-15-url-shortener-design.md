# URL Shortener — Design & Architecture

> Created: 2026-06-15 · Updated: 2026-06-16 · Status: Implemented
> A Go REST API that shortens URLs, redirects visitors, counts clicks, supports
> custom aliases and link expiration. Built with the standard library and a
> layered architecture as a portfolio + learning project.
>
> This document reflects the **current implementation**. It supersedes the
> original v1 plan.

## 1. Key Technical Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Web framework | **standard library `net/http`** | Go 1.22+ `ServeMux` does method+path routing; zero dependencies, easy to swap for chi later. |
| Storage | **MySQL** (via XAMPP) | A real RDBMS; also exercises Go's `database/sql`. |
| Short codes | **random base62** | Non-enumerable; uniqueness backed by a `UNIQUE` index + retry. |
| Click counting | **in-memory buffer flushed in batches** | Keeps the hot redirect path off the database; see §5. |
| Config | **env vars** (`.env` via godotenv) | 12-factor; the app reads `os.Getenv`, so production needs no `.env` file. |
| Charset / time | `utf8mb4`, **all timestamps in UTC** | Full Unicode; UTC on both driver and DB session avoids timezone drift. |

## 2. Architecture

Layered by technical role; dependencies point inward and are wired by hand in
`main` (no DI container).

```
cmd/server/main.go      composition root: build deps, start server, graceful shutdown
internal/
  handler/              HTTP layer: decode, validate, status codes, JSON  (≈ Controllers)
  shortener/            business logic: code generation, create/resolve/stats
  clicks/               in-memory click buffer + background flusher
  storage/              MySQL persistence via database/sql  (≈ Repository)
  model/                Link struct (data only, no Active Record)
  config/               environment configuration + DSN
migrations/             001 schema · 002 widen code · 003 add expires_at
```

**Interfaces are declared by the consumer** (idiomatic Go), so each layer
depends on a small abstraction it owns and can be tested with a fake:

- `shortener.Store` (CreateLink, FindByCode) and `shortener.ClickRecorder`
  (Incr, Pending) — implemented by `storage.LinkStore` and `clicks.Counter`.
- `clicks.Flusher` (AddClicks) — implemented by `storage.LinkStore`.
- `handler.Shortener` (Shorten, Resolve, Stats) — implemented by `*shortener.Service`.

One concrete type satisfies several small interfaces; compile-time assertions
(`var _ shortener.Store = (*LinkStore)(nil)`) catch signature drift early.

## 3. API

| Method & path | Purpose | Success | Errors |
|---|---|---|---|
| `GET /health` | liveness | `200` | — |
| `POST /shorten` | create a link (optional `alias`, `expires_in`) | `201` | `400` invalid, `409` alias taken |
| `GET /{code}` | redirect + count a click | `302` | `404` unknown, `410` expired |
| `GET /api/links/{code}/stats` | metadata + exact click count | `200` | `404` unknown |

Request body for `POST /shorten`:

```json
{ "url": "https://...", "alias": "my-code", "expires_in": 3600 }
```

`alias` and `expires_in` are optional. `alias` must be 3–32 chars of
`[A-Za-z0-9_-]` and not a reserved route word (`health`, `shorten`, `api`).

## 4. Short-Code Generation

`shortener.GenerateCode` builds a 7-character base62 string with **`crypto/rand`**
(not `math/rand`): codes must be unpredictable so links cannot be enumerated.
Each character is drawn uniformly via `rand.Int` to avoid modulo bias.

Uniqueness is a two-layer strategy:

1. **Source of truth:** a `UNIQUE` index on `links.code` — even under concurrency
   the DB never stores a duplicate.
2. **Application retry:** `Service.Shorten` inserts the generated code; on a
   duplicate-key error (MySQL 1062, translated to `ErrCodeExists`) it generates a
   fresh code and retries, up to 5 attempts. With a ~3.5×10¹² keyspace this is
   effectively never hit, but the cap prevents an infinite loop.

A **custom alias** skips generation and uses the code verbatim; a collision is a
hard `409 Conflict` (the caller asked for that exact code), not a retry.

## 5. Concurrency: Buffered Click Counting

`net/http` serves each request in its own goroutine sharing process memory, so
counting clicks in memory needs synchronization. `clicks.Counter` holds a
`map[string]int64` guarded by a `sync.Mutex`:

- **`Incr(code)`** (hot path, called from `Resolve`) takes the lock, bumps the
  count, returns — no I/O.
- A background goroutine (`Run`) **flushes** every 2s: it swaps the buffer out
  under the lock, then writes to MySQL *without* holding the lock (so `Incr`
  never blocks on the DB). `storage.AddClicks` applies the batch in a single
  transaction (`clicks = clicks + N` per code). A failed flush merges the counts
  back so nothing is lost.
- **Graceful shutdown:** on `SIGINT`/`SIGTERM`, `main` stops the HTTP server and
  flushes once more, bounding data loss to at most one interval on a hard crash.
- **Buffer-aware stats:** `Service.Stats` adds `Counter.Pending(code)` to the DB
  value, so the reported total is exact even before the next flush.

**Trade-off:** this trades a small amount of durability and strict consistency
(stats are eventually consistent; a hard crash loses ≤1 interval of counts) for
throughput — N visits to a hot link collapse into one `+N` write instead of N
row-locked `+1` writes.

## 6. Link Expiration

`POST /shorten` accepts an optional `expires_in` (seconds); the handler converts
it to an absolute `expires_at` (stored as nullable `TIMESTAMP`, `NULL` = never).
`Service.Resolve` returns `ErrGone` (→ `410 Gone`, not counted) when
`expires_at` is in the past. Expired rows are checked at read time, not purged.

## 7. Data Model

```sql
CREATE TABLE links (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  code        VARCHAR(32) NOT NULL UNIQUE,        -- random code or custom alias
  long_url    TEXT NOT NULL,
  clicks      BIGINT UNSIGNED NOT NULL DEFAULT 0,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at  TIMESTAMP NULL DEFAULT NULL         -- NULL = never expires
);
```

`code` started at `VARCHAR(16)` (001) and was widened to `VARCHAR(32)` (002) once
custom aliases could exceed 16 chars; `expires_at` was added in 003.

## 8. Configuration

Env vars (XAMPP-friendly defaults): `DB_HOST`, `DB_PORT`, `DB_USER`,
`DB_PASSWORD`, `DB_NAME`, `SERVER_ADDR`, `BASE_URL`.

The DSN sets `parseTime=true&charset=utf8mb4&loc=UTC&time_zone='+00:00'`. Pinning
**both** the driver (`loc`) and the MySQL session (`time_zone`) to UTC is
essential: if they disagree, `TIMESTAMP` values read back are off by the server's
offset, which silently breaks comparisons such as expiry.

## 9. Error Handling

No exceptions — errors are values. The layers translate low-level errors into
domain sentinels (`ErrCodeExists`, `ErrNotFound`, `ErrGone`); the HTTP layer maps
them with `errors.Is`:

| Sentinel / condition | HTTP status |
|---|---|
| invalid input | `400 Bad Request` |
| `ErrNotFound` | `404 Not Found` |
| `ErrGone` | `410 Gone` |
| `ErrCodeExists` (alias) | `409 Conflict` |
| unexpected | `500` (real cause logged server-side only) |

## 10. Testing

Unit tests cover code generation, the collision-retry logic, the concurrent
counter, and every handler (`httptest`) using hand-written fakes for the
consumer interfaces — fast and DB-free. End-to-end runs against real MySQL caught
two bugs the fakes could not: silent `VARCHAR(16)` truncation of long aliases,
and the driver/session timezone mismatch above.

## 11. Possible Future Work

- Background job to purge expired links (currently checked at read time only).
- Rate limiting and abuse protection.
- Per-user accounts / API keys.
