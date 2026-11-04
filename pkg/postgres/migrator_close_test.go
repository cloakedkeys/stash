//go:build integration

package postgres

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A migrator that fails to start closes its connection. The server it's
// given closes every connection straight away, so starting fails.
func TestNewMigratorFailureClosesConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	db := NewDatabase()
	db.dbPath = fmt.Sprintf("postgres://postgres@%s/stash?sslmode=disable&connect_timeout=2", listener.Addr())
	m, err := NewMigrator(db)
	require.Error(t, err)
	require.NotNil(t, m)
	require.NotNil(t, m.conn)
	assert.ErrorContains(t, m.conn.Ping(), "database is closed")
}
