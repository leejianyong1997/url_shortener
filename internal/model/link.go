package model

import "time"

// Link is one shortened-URL record — the data that lives in the `links` table.
//
// It is a PLAIN data struct: no save()/find()/update() methods, no Active
// Record. Unlike a Laravel Eloquent model, behavior does not live here.
// Reading/writing this to the database is the job of package storage.
type Link struct {
	ID        int64
	Code      string
	LongURL   string
	Clicks    int64
	CreatedAt time.Time
}
