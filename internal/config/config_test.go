package config

import (
	"strings"
	"testing"
)

// TestDSNPinsUTC guards against a timezone bug: if the driver and the MySQL
// session disagree on time zone, TIMESTAMP values read back are wrong (off by
// the server's UTC offset), which silently breaks time comparisons like link
// expiry. The DSN must force both sides to UTC.
func TestDSNPinsUTC(t *testing.T) {
	cfg := Config{DBUser: "u", DBPassword: "p", DBHost: "127.0.0.1", DBPort: "3306", DBName: "db"}
	dsn := cfg.DSN()

	if !strings.Contains(dsn, "parseTime=true") {
		t.Errorf("DSN must enable parseTime: %s", dsn)
	}
	if !strings.Contains(dsn, "loc=UTC") {
		t.Errorf("DSN must parse times in UTC (loc=UTC): %s", dsn)
	}
	if !strings.Contains(dsn, "time_zone=") {
		t.Errorf("DSN must pin the MySQL session time zone: %s", dsn)
	}
}
