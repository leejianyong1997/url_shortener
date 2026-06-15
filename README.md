# URL Shortener

A small URL shortener REST API written in Go and backed by MySQL. It turns a long
URL into a short code, redirects visitors to the original URL, and counts clicks.

Built with the standard library `net/http` (no web framework) and a layered
architecture.

## Highlights

- **Random base62 short codes** with uniqueness enforced by a `UNIQUE` index and
  an application-level retry on collision.
- **High-throughput click counting**: visits are aggregated in memory behind a
  `sync.Mutex` and flushed to MySQL in batches by a background goroutine, so the
  redirect path never blocks on a database write. Counts are eventually
  consistent (flushed every ~2s).
- **Graceful shutdown**: on `SIGINT`/`SIGTERM` the server drains in-flight
  requests and flushes buffered clicks before exiting.
- **Layered design**: `handler` (HTTP) → `shortener` (business logic) →
  `storage` (MySQL), wired by hand in `main` (no DI container).

## Tech Stack

- Go 1.22+ (developed on 1.26)
- MySQL 8 / MariaDB (e.g. via XAMPP)
- [`go-sql-driver/mysql`](https://github.com/go-sql-driver/mysql),
  [`joho/godotenv`](https://github.com/joho/godotenv)

## Getting Started

### Prerequisites

- Go 1.22 or newer
- A running MySQL/MariaDB server (XAMPP works out of the box)

### 1. Clone

```bash
git clone https://github.com/leejianyong1997/url_shortener.git
cd url_shortener
```

### 2. Create the database and table

Run the migration once. It creates the `url_shortener` database and the `links`
table.

```bash
mysql -u root < migrations/001_create_links.sql
```

> XAMPP on Windows: use the bundled client, e.g.
> `"C:\xampp\mysql\bin\mysql.exe" -u root < migrations/001_create_links.sql`,
> or paste the file into phpMyAdmin's SQL tab.

### 3. Configure (optional)

The defaults target a stock XAMPP MySQL (`root`, empty password). To override,
copy the example file and edit it:

```bash
cp .env.example .env
```

| Variable      | Default                 | Description                     |
| ------------- | ----------------------- | ------------------------------- |
| `DB_HOST`     | `127.0.0.1`             | MySQL host                      |
| `DB_PORT`     | `3306`                  | MySQL port                      |
| `DB_USER`     | `root`                  | MySQL user                      |
| `DB_PASSWORD` | _(empty)_               | MySQL password                  |
| `DB_NAME`     | `url_shortener`         | Database name                   |
| `SERVER_ADDR` | `:8080`                 | Address the HTTP server listens on |
| `BASE_URL`    | `http://localhost:8080` | Base used to build short URLs    |

### 4. Run

```bash
go run ./cmd/server
```

The server starts on `http://localhost:8080`. Press `Ctrl+C` to stop it
gracefully.

## API

Base URL: `http://localhost:8080`

### `GET /health`

Liveness check.

```bash
curl http://localhost:8080/health
```

`200 OK`

```json
{ "status": "ok" }
```

---

### `POST /shorten`

Create a short link for a long URL.

**Request body**

| Field | Type   | Required | Description                          |
| ----- | ------ | -------- | ------------------------------------ |
| `url` | string | yes      | Absolute `http`/`https` URL to shorten |

```bash
curl -X POST http://localhost:8080/shorten \
  -H "Content-Type: application/json" \
  -d '{"url":"https://go.dev/doc/effective_go"}'
```

`201 Created`

```json
{
  "code": "QvxNEc8",
  "short_url": "http://localhost:8080/QvxNEc8",
  "long_url": "https://go.dev/doc/effective_go"
}
```

**Validation errors** — `400 Bad Request`:

```json
{ "error": "field 'url' must be a valid http(s) URL" }
```

| Condition                 | Status | Message                              |
| ------------------------- | ------ | ------------------------------------ |
| Body is not valid JSON    | 400    | `request body must be valid JSON`    |
| `url` missing or empty    | 400    | `field 'url' is required`            |
| `url` not a valid http(s) | 400    | `field 'url' must be a valid http(s) URL` |

---

### `GET /{code}`

Resolve a short code and redirect to the original URL. Each visit increments the
click counter (buffered, see Highlights).

```bash
# -i shows the redirect; -L would follow it to the destination
curl -i http://localhost:8080/QvxNEc8
```

`302 Found`

```
HTTP/1.1 302 Found
Location: https://go.dev/doc/effective_go
```

A `302` (temporary) redirect is used deliberately rather than `301`: browsers
cache `301`s aggressively, which would stop visits from reaching the server and
silently break click counting.

**Unknown code** — `404 Not Found`:

```json
{ "error": "short link not found" }
```

---

### `GET /api/links/{code}/stats`

Return a code's metadata and click count **without** counting a visit. The total
is exact: it includes clicks still buffered in memory that have not yet been
flushed to the database.

```bash
curl http://localhost:8080/api/links/QvxNEc8/stats
```

`200 OK`

```json
{
  "code": "QvxNEc8",
  "short_url": "http://localhost:8080/QvxNEc8",
  "long_url": "https://go.dev/doc/effective_go",
  "clicks": 5,
  "created_at": "2026-06-15T18:59:35Z"
}
```

**Unknown code** — `404 Not Found`:

```json
{ "error": "short link not found" }
```

## Running the Tests

```bash
go test ./...
```

The suite includes unit tests for code generation, the collision-retry logic
(via a fake store), the HTTP handlers (`httptest`), and a concurrency test that
proves click counts are not lost under parallel access.

## Project Layout

```
cmd/server/        program entry point and composition root
internal/
  handler/         HTTP layer: decode, validate, redirect, JSON responses
  shortener/       business logic: base62 codes, collision retry, resolve
  storage/         MySQL persistence (database/sql)
  clicks/          in-memory click buffer + background flusher
  model/           shared data structs (Link)
  config/          environment configuration
migrations/        SQL schema
```

## Roadmap

- Custom aliases (user-chosen codes).
- Link expiration.
