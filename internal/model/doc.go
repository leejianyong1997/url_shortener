// Package model holds the plain data structures shared across the app
// (e.g. Link). These are data-only structs: no persistence logic and no
// Active Record — unlike a Laravel Eloquent model, a Go struct here has no
// save()/find() methods. Persistence lives in package storage instead.
package model
