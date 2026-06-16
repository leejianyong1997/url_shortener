# URL Shortener — Design Document (v1)

> Date: 2026-06-15 · Status: Approved, implementation started
> Author background: Several years of PHP/Laravel backend experience; Go is a new
> language being learned. This is a portfolio + learning project.

## 1. Goal

A URL shortener REST API written in Go. Scope of the first usable version (v1):

- `POST /api/links` — submit a long URL, get back a short code
- `GET /{code}` — 302 redirect to the original URL and increment the click count
- `GET /api/links/{code}/stats` — view click statistics for a link
- `GET /health` — health check (also the "hello world" of step 1)

## 2. Key Technical Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Web framework | **standard library `net/http`** | Go 1.22+ `ServeMux` already does method+path routing; zero dependencies, highest learning value; easy to swap for chi later (chi is net/http underneath). |
| Storage | **MySQL (XAMPP)** | Closest to production; also a chance to learn Go's `database/sql`. |
| Short-code generation | **random base62** | 6–7 chars of `[a-zA-Z0-9]`, retry on collision; non-enumerable, simple to implement. |
| v1 scope | **core + click stats** | Realistic enough to feel like a real service without ballooning. |
| Database name | `url_shortener` | Same as the project; all lowercase snake_case (cross-platform safe). |
| Charset | `utf8mb4` | Full Unicode (incl. emoji); avoids the legacy 3-byte `utf8` trap. |

## 3. Directory Structure

```
url_shortener/
├── go.mod                      # module name + dependencies (≈ composer.json)
├── go.sum                      # dependency checksums (≈ composer.lock)
├── .env.example                # DB connection template
├── cmd/
│   └── server/
│       └── main.go             # entry point / composition root: wires deps by hand
├── internal/                   # private packages; the compiler forbids imports from outside the module
│   ├── handler/                # HTTP layer  (≈ Controllers)
│   ├── shortener/              # business logic (≈ Services/Actions)
│   ├── storage/                # MySQL data access (≈ Repository)
│   └── model/                  # plain data structs: Link, etc. (≈ Models, but data-only, no Active Record)
└── migrations/
    └── 001_create_links.sql    # raw SQL schema
```

**Layering philosophy:** layered by technical role (handler/shortener/storage),
which maps cleanly onto the Laravel mental model. The other common Go style is
"package by feature" (a single `links` package); for a service this small either
works.

## 4. Data Flow

```
POST /api/links {"url":"..."}
  handler.CreateLink → shortener.Shorten(url) → storage.Save(link) → MySQL
  ← 201 {"short_url":"http://localhost:8080/abc123X"}

GET /{code}
  handler.Redirect → storage.FindByCode → storage.IncrementClicks → 302 redirect

GET /api/links/{code}/stats
  handler.Stats → storage.FindByCode → 200 {"url":...,"clicks":42,"created_at":...}

GET /health → 200 OK
```

## 5. MySQL Schema

```sql
CREATE DATABASE IF NOT EXISTS url_shortener
  CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

CREATE TABLE links (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  code        VARCHAR(16) NOT NULL UNIQUE,
  long_url    TEXT NOT NULL,
  clicks      BIGINT UNSIGNED NOT NULL DEFAULT 0,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

## 6. Configuration (XAMPP defaults)

```
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASSWORD=          # XAMPP default: root with no password (always change in production)
DB_NAME=url_shortener
```

## 7. Implementation Phases

1. **Foundation (this step):** go module, directory skeleton, `GET /health` running.
2. Configuration loading + MySQL connection (`database/sql`).
3. model + storage layer (including migration).
4. shortener (random base62) + `POST /api/links`.
5. `GET /{code}` redirect + click increment.
6. `GET /api/links/{code}/stats`.
7. Tests, README, error-handling polish.
