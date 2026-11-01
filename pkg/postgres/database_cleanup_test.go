//go:build integration

package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// Only use this for uniquely named throwaway databases. A failed drop cannot
// affect later tests, and CI discards the PostgreSQL service after the job.
func dropTestDatabase(t *testing.T, name string, drop func() error) {
	t.Helper()
	if err := drop(); err != nil {
		t.Logf("cleanup: could not drop throwaway database %q: %v", name, err)
	}
}

func TestDropTestDatabase(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "success"},
		{name: "permission_denied", err: &pgconn.PgError{Code: "42501", Message: "permission denied to terminate process"}},
	} {
		called := 0
		continued := false
		passed := t.Run(tc.name, func(t *testing.T) {
			dropTestDatabase(t, "throwaway_123", func() error {
				called++
				return tc.err
			})
			continued = true
		})
		require.True(t, passed, "cleanup must not fail the test")
		require.True(t, continued, "cleanup must return even when the drop fails")
		require.Equal(t, 1, called, "cleanup attempts the drop once")
	}
}
