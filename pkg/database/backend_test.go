package database

import "testing"

func TestIsPostgresPath(t *testing.T) {
	tests := map[string]bool{
		"":                                     false,
		"stash-go.sqlite":                      false,
		"/stash/postgres.sqlite":               false,
		"postgresql://user:pw@host:5432/db":    true,
		"postgres://user:pw@host/db":           true,
		"POSTGRESQL:postgresql://user@host/db": true,
		"postgresql:host=h dbname=d":           true,
	}
	for path, want := range tests {
		if got := IsPostgresPath(path); got != want {
			t.Errorf("IsPostgresPath(%q) = %v, want %v", path, got, want)
		}
	}
}
