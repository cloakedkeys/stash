//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A custom field value reads as the same text here as on SQLite, so includes,
// excludes and regex filters find the same items on both databases (#2056).
// Each value is stored as the store stores it, and its text compared with
// what SQLite reads for the value its store keeps.
func TestCustomFieldTextMatchesSQLite(t *testing.T) {
	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		t.Skip("PGSQL_TEST is required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(ctx) })
	tx, err := conn.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	lite, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lite.Close() })
	lite.SetMaxOpenConns(1)
	_, err = lite.Exec("CREATE TABLE cf (value BLOB NOT NULL)")
	require.NoError(t, err)

	_, err = tx.Exec(ctx, "CREATE TEMP TABLE cf_text (value JSONB NOT NULL, type TEXT)")
	require.NoError(t, err)

	values := []interface{}{
		// the examples in #2056
		float64(1.0), float64(1e20), float64(0.00001), true, false,
		// floats around where SQLite changes between the plain and the
		// exponent form, and with up to 13 significant digits
		float64(0), float64(-2), float64(-0.5), float64(0.1), float64(1.5), float64(4.56),
		float64(123.456), float64(0.0001), float64(0.000125), float64(1.25e-5), float64(-1e-7),
		float64(1e15), float64(1e16), float64(12345678901234567), float64(1e17), float64(1e21),
		float64(1.5e300), float64(1e100), float64(1e-100), float64(1234567890123),
		float64(99999999999.5), float64(0.3), float64(2.5e-5),
		// read the same on both already
		int64(0), int64(5), int64(-12), int64(9007199254740993), "text", "1.0", "true", "",
	}

	for _, v := range values {
		t.Run(fmt.Sprintf("%T %v", v, v), func(t *testing.T) {
			_, err := tx.Exec(ctx, "DELETE FROM cf_text")
			require.NoError(t, err)
			pv, err := getSQLValueFromCustomFieldInput(v)
			require.NoError(t, err)
			_, err = tx.Exec(ctx, "INSERT INTO cf_text (value, type) VALUES ($1::JSONB, $2)", pv, getSQLTypeFromInput(v))
			require.NoError(t, err)
			var got string
			require.NoError(t, tx.QueryRow(ctx, "SELECT "+customFieldText("cf")+" FROM cf_text AS cf").Scan(&got))

			_, err = lite.Exec("DELETE FROM cf")
			require.NoError(t, err)
			_, err = lite.Exec("INSERT INTO cf (value) VALUES (?)", v)
			require.NoError(t, err)
			var want string
			require.NoError(t, lite.QueryRow("SELECT CAST(value AS TEXT) FROM cf").Scan(&want))

			assert.Equal(t, want, got)
		})
	}
}

func TestCustomFieldSearchText(t *testing.T) {
	tests := []struct {
		in   interface{}
		want string
	}{
		{json.Number("1.0"), "1"},
		{json.Number("5"), "5"},
		{json.Number("0.00001"), "1e-05"},
		{json.Number("1e20"), "1e+20"},
		{float64(2.5), "2.5"},
		{"1.0", "1.0"},
		{true, "true"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, customFieldSearchText(tt.in), "%v", tt.in)
	}
}
