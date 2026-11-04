//go:build integration

package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A connection pool that can't be opened reports the error instead of crashing.
func TestOpenDBReportsOpenError(t *testing.T) {
	const driverName = "not-a-registered-driver"

	db := NewDatabase()
	db.dbPath = filepath.Join(t.TempDir(), "stash-go.sqlite")

	assert.NotPanics(t, func() {
		assert.ErrorContains(t, db.openReadDBWithDriver(driverName), "db.Open()")
	}, "read")
	assert.NotPanics(t, func() {
		assert.ErrorContains(t, db.openWriteDBWithDriver(driverName), "db.Open()")
	}, "write")
}
