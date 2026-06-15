package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/leejianyong1997/url_shortener/internal/clicks"
	"github.com/leejianyong1997/url_shortener/internal/model"
	"github.com/leejianyong1997/url_shortener/internal/shortener"
)

// LinkStore persists links in MySQL. It implements shortener.Store, so the
// shortener.Service can stay ignorant of SQL entirely.
type LinkStore struct {
	db *sql.DB
}

// NewLinkStore returns a LinkStore backed by the given connection pool.
func NewLinkStore(db *sql.DB) *LinkStore {
	return &LinkStore{db: db}
}

// Compile-time assertions that *LinkStore satisfies the interfaces its
// consumers declare. One concrete type, several small interfaces — idiomatic Go.
var (
	_ shortener.Store = (*LinkStore)(nil)
	_ clicks.Flusher  = (*LinkStore)(nil)
)

// CreateLink inserts a new row. Note the `?` placeholders: the driver sends the
// values separately from the SQL text, so this is a parameterized query and is
// immune to SQL injection — same idea as PDO prepared statements.
//
// A duplicate `code` trips the UNIQUE index and MySQL returns error 1062. We
// translate ONLY that specific case into shortener.ErrCodeExists, so the
// service knows it was a collision (retry) versus a real failure (give up).
func (s *LinkStore) CreateLink(ctx context.Context, link *model.Link) error {
	const q = `INSERT INTO links (code, long_url, expires_at) VALUES (?, ?, ?)`
	res, err := s.db.ExecContext(ctx, q, link.Code, link.LongURL, nullTime(link.ExpiresAt))
	if err != nil {
		var myErr *mysql.MySQLError
		if errors.As(err, &myErr) && myErr.Number == 1062 {
			return shortener.ErrCodeExists
		}
		return fmt.Errorf("insert link: %w", err)
	}
	// MySQL hands back the auto-increment id; fill it into the struct.
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	link.ID = id
	return nil
}

// FindByCode loads one link by its short code. database/sql has no result rows
// when the code is unknown, surfaced as sql.ErrNoRows; we translate that into
// the domain error shortener.ErrNotFound so upper layers don't depend on the
// sql package.
func (s *LinkStore) FindByCode(ctx context.Context, code string) (*model.Link, error) {
	const q = `SELECT id, code, long_url, clicks, created_at, expires_at FROM links WHERE code = ?`
	var link model.Link
	// expires_at is nullable, so we scan it into a sql.NullTime and convert.
	var expiresAt sql.NullTime
	// QueryRowContext + Scan copies each column into a Go field BY POSITION.
	// You pass pointers (&link.ID, ...) so Scan can write into them.
	err := s.db.QueryRowContext(ctx, q, code).Scan(
		&link.ID, &link.Code, &link.LongURL, &link.Clicks, &link.CreatedAt, &expiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shortener.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find link: %w", err)
	}
	if expiresAt.Valid {
		link.ExpiresAt = &expiresAt.Time
	}
	return &link, nil
}

// nullTime converts an optional *time.Time into a sql.NullTime so a nil pointer
// is stored as SQL NULL.
func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// AddClicks applies a batch of buffered counts in a single transaction: one
// `clicks = clicks + N` UPDATE per code. This is the payoff of buffering —
// many redirects on the same code collapse into one +N write instead of N
// separate +1 writes, and they all commit atomically.
//
// Transactions are how database/sql groups statements: BeginTx -> Exec... ->
// Commit. The `defer tx.Rollback()` is the idiomatic safety net — if we return
// early on an error, it rolls back; after a successful Commit it's a no-op.
func (s *LinkStore) AddClicks(ctx context.Context, counts map[string]int64) error {
	if len(counts) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	const q = `UPDATE links SET clicks = clicks + ? WHERE code = ?`
	for code, n := range counts {
		if _, err := tx.ExecContext(ctx, q, n, code); err != nil {
			return fmt.Errorf("add clicks for %q: %w", code, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit clicks: %w", err)
	}
	return nil
}
