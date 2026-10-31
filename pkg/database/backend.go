package database

import "strings"

// IsPostgresPath reports whether a database setting names a PostgreSQL server
// rather than an SQLite file. Both postgresql:// and postgres:// addresses work.
func IsPostgresPath(path string) bool {
	upper := strings.ToUpper(path)
	return strings.HasPrefix(upper, string(PostgresBackend)+":") || strings.HasPrefix(upper, "POSTGRES://")
}
