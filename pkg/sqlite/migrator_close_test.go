//go:build integration

package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A migrator that fails to start closes its connection.
func TestNewMigratorFailureClosesConnection(t *testing.T) {
	db := NewDatabase()
	db.dbPath = filepath.Join(t.TempDir(), "missing", "stash-go.sqlite")
	m, err := NewMigrator(db)
	require.Error(t, err)
	require.NotNil(t, m)
	require.NotNil(t, m.conn)
	assert.ErrorContains(t, m.conn.Ping(), "database is closed")
}
