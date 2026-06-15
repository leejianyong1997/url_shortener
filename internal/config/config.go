// Package config loads runtime configuration from environment variables.
//
// In Laravel you read config via config('database.connections.mysql.host'),
// which is wired up by the framework. Go has no framework, so we read the
// environment ourselves once at startup into a plain struct and pass it down.
package config

import (
	"fmt"
	"os"
)

// Config holds all runtime settings, read once at startup.
type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	ServerAddr string
	BaseURL    string
}

// Load reads configuration from environment variables, falling back to
// sensible local-dev (XAMPP) defaults when a variable is unset.
func Load() Config {
	return Config{
		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "url_shortener"),
		ServerAddr: getEnv("SERVER_ADDR", ":8080"),
		BaseURL:    getEnv("BASE_URL", "http://localhost:8080"),
	}
}

// DSN builds the MySQL data source name that database/sql needs.
//
//   - parseTime=true scans DATETIME/TIMESTAMP straight into time.Time.
//   - loc=UTC parses those times as UTC, and time_zone='+00:00' forces the
//     MySQL SESSION to UTC too. Both sides MUST agree: if the server session is
//     in a non-UTC zone (e.g. +08) while the driver assumes UTC, every time read
//     back is wrong by the offset, which silently breaks comparisons like link
//     expiry. (%%27 and %%2B are URL-encoded ' and + inside the DSN.)
func (c Config) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=UTC&time_zone=%%27%%2B00%%3A00%%27",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

// getEnv returns the value of key, or fallback if it is unset or empty.
func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
