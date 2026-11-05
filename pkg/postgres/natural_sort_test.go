//go:build integration

package postgres

import (
	"context"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/WithoutPants/sortorder/casefolded"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// #2120: a name sort on PostgreSQL orders names exactly as SQLite's
// NATURAL_CI collation, casefolded.NaturalLess, does. For every pair of names,
// their sort keys compare the way NaturalLess compares the names, including
// names it finds equal.
func TestNaturalSortKeyMatchesSQLite(t *testing.T) {
	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		t.Skip("PGSQL_TEST is required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(ctx) })

	names := []string{
		"", " ", "a", "A", "b", "B", "_", "-", "~", ".", "0", "00", "000", "1", "01", "001", "10", "010", "9", "09",
		"Tied Title", "tied title", "TIED TITLE", "Tied title 2", "Tied title 10", "Tied title 02",
		"Scene 1", "Scene 01", "Scene 1a", "Scene 1 a", "Scene1", "scene 2", "Scene 10", "Scene 100", "Scene 99",
		"Café", "CAFÉ", "cafe", "Cafe", "Cafè", "Straße", "STRASSE", "strasse", "Ελληνικά", "ΕΛΛΗΝΙΚΆ",
		"Pokémon", "POKÉMON", "İstanbul", "istanbul", "ıstanbul", "Kelvin K", "Kelvin K", "Kelvin k",
		"日本語", "日本語 2", "日本語 10", "😀", "😀 1", "a😀", "Ǆ", "ǅ", "ǆ", "ſ", "s", "S", "µ", "Μ", "μ",
		"á", "á", "x-1", "x_1", "x 1", "x.1", "x1", "x01.5", "x1.05", "v1.2.10", "v1.10.2", "v1.2.9",
		"Part 3 of 12", "Part 12 of 3", "123456789012345678901234567890", "123456789012345678901234567891",
		"0000000000000000000001", "1e10", "1E10", "[tag]", "(tag)", "{tag}", "Zz", "zZ", "ZZ", "zz",
		"– dash", "— dash", "- dash", "‘quote’", "'quote'", " nbsp", "tab\tname",
		// equal ignoring case but of different lengths in UTF-8 before a
		// number: NaturalLess orders equal numbers by their byte position
		"k0", "K0", "K0", "k00", "K00", "k0ς", "ß3", "ẞ3", "ß03", "ẞ03",
		"ſ1", "s1", "S1", "K1Aa", "K1本", "Ω 2", "Ω 2", "ω 2",
	}
	// random names from the same kinds of characters, to find pairs the list
	// above doesn't think of
	alphabet := []rune("aAbBzZsS09123 0_-.~éÉßẞΣσςİıiIKkKſΩωΩåÅÅǅǄǆ日本😀– ")
	r := rand.New(rand.NewSource(2120))
	for n := 0; n < 3000; n++ {
		var b strings.Builder
		for l := r.Intn(8); l >= 0; l-- {
			b.WriteRune(alphabet[r.Intn(len(alphabet))])
		}
		names = append(names, b.String())
	}

	tables := naturalFoldTables()
	sql := "WITH " + naturalFoldCTE + " AS (SELECT CAST($1 AS text) AS cased, CAST($2 AS text) AS fold_from, CAST($3 AS text) AS fold_to)" +
		" SELECT n, " + naturalSortKey("n") + " FROM unnest(CAST($4 AS text[])) AS n"
	rows, err := conn.Query(ctx, sql, tables.cased, tables.from, tables.to, names)
	require.NoError(t, err)
	keys := map[string]string{}
	for rows.Next() {
		var n, k string
		require.NoError(t, rows.Scan(&n, &k))
		keys[n] = k
	}
	require.NoError(t, rows.Err())
	rows.Close()

	var unique []string
	for n := range keys {
		unique = append(unique, n)
	}
	require.Greater(t, len(unique), 1000)

	natural := func(a, b string) int {
		switch {
		case casefolded.NaturalLess(a, b):
			return -1
		case casefolded.NaturalLess(b, a):
			return 1
		}
		return 0
	}

	wrong := 0
	for i, a := range unique {
		for _, b := range unique[i+1:] {
			want := natural(a, b)
			if got := strings.Compare(keys[a], keys[b]); got != want {
				wrong++
				if wrong <= 20 {
					t.Errorf("%+q vs %+q: SQLite gives %d, key gives %d (keys %+q, %+q)", a, b, want, got, keys[a], keys[b])
				}
			}
		}
	}
	if wrong > 0 {
		t.Errorf("%d of %d pairs sort differently", wrong, len(unique)*(len(unique)-1)/2)
	}
}

// An empty name sorts as an empty key, not as no key, so it comes after names
// that have no value, as on SQLite.
func TestNaturalSortKeyNull(t *testing.T) {
	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		t.Skip("PGSQL_TEST is required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(ctx) })

	tables := naturalFoldTables()
	sql := "WITH " + naturalFoldCTE + " AS (SELECT CAST($1 AS text) AS cased, CAST($2 AS text) AS fold_from, CAST($3 AS text) AS fold_to)" +
		" SELECT " + naturalSortKey("CAST(NULL AS text)") + " IS NULL, " + naturalSortKey("''") + " = ''"
	var nullIsNull, emptyIsEmpty bool
	require.NoError(t, conn.QueryRow(ctx, sql, tables.cased, tables.from, tables.to).Scan(&nullIsNull, &emptyIsEmpty))
	require.True(t, nullIsNull, "no name gives no key")
	require.True(t, emptyIsEmpty, "an empty name gives an empty key")
}
