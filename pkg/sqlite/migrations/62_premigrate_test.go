//go:build integration

package migrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stretchr/testify/require"
)

// databaseAt returns the path of a database built the normal way to
// version, with rows added by setup.
func databaseAt(t *testing.T, version uint, setup string) string {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(dir, "config.yml"))
	require.NoError(t, cfg.Write())

	path := filepath.Join(dir, "stash.sqlite")
	raw, err := sqlx.Open("sqlite3", path)
	require.NoError(t, err)
	defer raw.Close()
	initial, err := os.ReadFile("1_initial.up.sql")
	require.NoError(t, err)
	_, err = raw.Exec(string(initial) + "\nCREATE TABLE schema_migrations(version uint64, dirty bool); INSERT INTO schema_migrations VALUES(1,false);")
	require.NoError(t, err)

	db := sqlite.NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	m, err := sqlite.NewMigrator(db)
	require.NoError(t, err)
	for v := uint(2); v <= version; v++ {
		require.NoErrorf(t, m.RunMigration(ctx, v), "schema %d", v)
	}
	m.Close()
	require.NoError(t, db.Close())

	_, err = raw.Exec(setup)
	require.NoError(t, err)
	return path
}

// upgradeToLatest upgrades the database as the upgrade task does.
func upgradeToLatest(t *testing.T, path string) error {
	t.Helper()
	ctx := context.Background()
	db := sqlite.NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	defer db.Close()
	m, err := sqlite.NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	for m.CurrentSchemaVersion() < m.RequiredSchemaVersion() {
		v := m.CurrentSchemaVersion() + 1
		if err := m.RunMigration(ctx, v); err != nil {
			return fmt.Errorf("schema %d: %w", v, err)
		}
	}
	return nil
}

func queryStrings(t *testing.T, path string, query string) []string {
	t.Helper()
	raw, err := sqlx.Open("sqlite3", path)
	require.NoError(t, err)
	defer raw.Close()
	var out []string
	require.NoError(t, raw.Select(&out, query))
	return out
}

// warnLogger keeps the warnings logged.
type warnLogger struct {
	logger.BasicLogger
	mu       sync.Mutex
	warnings []string
}

func (l *warnLogger) Warnf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnings = append(l.warnings, fmt.Sprintf(format, args...))
}

func captureWarnings(t *testing.T) *warnLogger {
	l := &warnLogger{}
	previous := logger.Logger
	logger.Logger = l
	t.Cleanup(func() { logger.Logger = previous })
	return l
}

// A performer with no name doesn't stop the upgrade: it is kept, with a
// placeholder name and its ID, and the log says so (#2244).
func TestPre62NamesUnnamedPerformers(t *testing.T) {
	path := databaseAt(t, 61, `
INSERT INTO performers(id,name,created_at,updated_at) VALUES
  (1,'Kim',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (2,NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (3,NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (4,'Unnamed performer 3',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`)
	log := captureWarnings(t)

	require.NoError(t, upgradeToLatest(t, path))

	require.Equal(t, []string{
		"1 Kim",
		"2 Unnamed performer 2",
		// that name was taken
		"3 Unnamed performer 3 (2)",
		"4 Unnamed performer 3",
	}, queryStrings(t, path, "SELECT id || ' ' || name FROM performers ORDER BY id"))

	var renamed []string
	for _, w := range log.warnings {
		if strings.HasPrefix(w, "Performer ") && strings.Contains(w, "has no name") {
			renamed = append(renamed, w)
		}
	}
	require.Equal(t, []string{
		`Performer 2 has no name, which is now required. Naming it "Unnamed performer 2".`,
		`Performer 3 has no name, which is now required. Naming it "Unnamed performer 3 (2)".`,
	}, renamed)
}

// Without performers that have no name, the names don't change.
func TestPre62KeepsNames(t *testing.T) {
	path := databaseAt(t, 61, `
INSERT INTO performers(id,name,disambiguation,created_at,updated_at) VALUES
  (1,'Kim',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (2,'Kim','1',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (3,'',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`)

	require.NoError(t, upgradeToLatest(t, path))

	require.Equal(t, []string{"1 Kim ", "2 Kim 1", "3  "},
		queryStrings(t, path, "SELECT id || ' ' || name || ' ' || COALESCE(disambiguation, '') FROM performers ORDER BY id"))
}
