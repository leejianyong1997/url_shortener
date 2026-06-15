package storage

import (
	"database/sql"
	"fmt"
	"time"

	// Blank import: we never call this package directly. Importing it only for
	// its side effect runs its init(), which registers the "mysql" driver name
	// with database/sql. This is THE idiomatic Go way to plug in a SQL driver.
	_ "github.com/go-sql-driver/mysql"
)

// Connect opens a MySQL connection pool and verifies it is reachable.
func Connect(dsn string) (*sql.DB, error) {
	// sql.Open does NOT actually connect — it validates the DSN and prepares
	// the pool. The first real connection happens lazily (here, on Ping).
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	// Pool tuning. Big difference from PHP/PDO: there, each request opens its
	// own connection and throws it away at the end. A Go *sql.DB is one
	// long-lived pool shared by every goroutine for the life of the process.
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Ping forces a real round-trip so we fail fast at startup if the DB is down.
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return db, nil
}
