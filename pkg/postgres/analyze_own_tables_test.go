//go:build integration

package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2203: ANALYZE and the Optimise task's VACUUM only cover stash's own tables,
// so a lock that another database on the same server holds on a catalog shared
// by every database, such as pg_authid, doesn't make them wait.
func TestAnalyzeSkipsSharedCatalogs(t *testing.T) {
	ctx := context.Background()
	config.InitializeEmpty()
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, migrationFixture(t)))
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	// Hold the lock a plain ANALYZE or VACUUM of pg_authid takes. It only
	// conflicts with those (and index builds), not with logins.
	other, err := sqlx.Open("pgx", os.Getenv("PGSQL_TEST"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, other.Close()) })
	lock, err := other.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, lock.Rollback()) }()
	if _, err := lock.Exec("LOCK TABLE pg_catalog.pg_authid IN SHARE UPDATE EXCLUSIVE MODE"); err != nil {
		t.Skipf("can't lock pg_authid as this user: %v", err)
	}

	for name, run := range map[string]func(context.Context) error{
		"analyze":      db.Analyze,
		"vacuum":       db.Vacuum,
		"post migrate": func(ctx context.Context) error { return analyze(ctx, db.writeDB) },
	} {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		start := time.Now()
		err := run(ctx)
		cancel()
		assert.NoError(t, err, name)
		assert.Less(t, time.Since(start), 5*time.Second, name)
	}
}

// The list ANALYZE and VACUUM use covers every stash table and nothing
// outside its schema.
func TestOwnTablesQuery(t *testing.T) {
	ctx := context.Background()
	config.InitializeEmpty()
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, migrationFixture(t)))
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	tables, err := ownTables(ctx, db.writeDB)
	require.NoError(t, err)
	var want []string
	require.NoError(t, db.writeDB.SelectContext(ctx, &want, `SELECT format('%I.%I', table_schema, table_name) FROM information_schema.tables
WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' ORDER BY table_name`))
	assert.Equal(t, strings.Join(want, ", "), tables)

	var schema string
	require.NoError(t, db.writeDB.GetContext(ctx, &schema, "SELECT current_schema()"))
	assert.Contains(t, want, schema+".scenes")
	assert.NotContains(t, tables, "pg_authid")
}
