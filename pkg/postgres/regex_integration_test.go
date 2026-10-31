//go:build integration

package postgres

import (
	"context"
	"math/rand"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/txn"
)

var regexTestStrings = []string{
	"", "foo", "FOO", "Foo", "foo bar", "foobar", "fooBAR", "xfoo", "barfoo", "bar\nfoo", "foo\nbar", "foo\n", "\nfoo", "a\nb", "a b", "a", "b", "aa", "aaa", "ab", "abab", "ababab", "A", "\n", "\r\n",
	"Foo (2020)", "Foo 2020", "2020", "/data/videos/a.mp4", "/data/x.MKV", "/data/x.mkv", "é", "É", "über foo", "é foo", "fooé", "Straße", "STRASSE", "ß", "ẞ", "K", "k", "K", "s", "S", "ſ", "σ", "Σ", "ς",
	"日本語", "日本語 foo", "αβγ", "привет", "😀", "a.b", "axb", "$", `\`, "]", "-", "\t", "١٢٣", "foo_bar", "howwomenorgasm.com", "switch.com", "Foo & Bar",
}

func regexTestPatterns() []string {
	patterns := []string{
		``, `foo`, `(?i)foo`, `foo(?i)bar`, `(?i)foo(?-i)bar`, `(?P<y>\d{4})`, `(?P<a>fo+)`, `\pL+`, `^\pL+$`, `\p{Greek}`, `\P{Greek}`, `\p{Cyrillic}+`, `\p{Han}`,
		`\d+`, `^\d+$`, `\D`, `\w+`, `^\w+$`, `\W`, `\s`, `\S`, `\bfoo\b`, `\Bfoo`, `foo\B`, `\b`, `\B`, `^foo$`, `^$`, `$`, `^`, `(?m)^bar$`, `(?m)^foo`, `(?m)foo$`, `foo$`, `(?s)a.b`, `a.b`, `.`, `^.$`, `.*`, `^.*$`,
		`\Afoo\z`, `foo\z`, `a{2,3}`, `^a{2}$`, `a{2,}`, `a{1000}`, `a{0,1000}`, `(a|b){0,1000}`, `a{500,1000}`, `(ab?){0,500}`, `\pL{0,50}`, `(\pL\d){20}`, `(a|b){200}`, `(?:ab){3}`, `x*?y`, `a+?`, `(?U)a+`, `a??`, `[[:alpha:]]+`, `[[:digit:]]`, `[[:^alpha:]]`, `[[:word:]]+`, `[^a-z]`, `[\d-]`, `[a-]`, `[\]]`, `[^\n]`, `\Qa.b\E`, `\x41`, `\x{1F600}`, `\x00`, `[\x00]`, `\101`,
		`é`, `(?i)É`, `(?i)ß`, `(?i)k`, `(?i)s`, `(?i)σ`, `(?i)straße`, `\.mp4$`, `(?i)\.mp4$`, `.*\.(mp4|mkv)$`, `MKV|MP4`, `^/data/.*`, `(a+)+$`, `\$`, `\\`, `\t`, `\n`, `\r?\n`, `日本語`, `[а-я]+`, `😀`, `[😀]`, `\(2020\)`, `Foo \(2020\)`, `Foo (2020)`, `Foo.*`, `Foo & Bar`, `a|`, `|a`, `()`, `(|a)`, `(a)(b)?`, `[^\x00-\x{10FFFF}]`, `howwomenorgasm\.com|switch\.com|getupclose\.com`,
	}

	atoms := []string{`a`, `b`, `foo`, `\d`, `\w`, `\s`, `.`, `[a-c]`, `[^a]`, `(?i)f`, `\b`, `\B`, `^`, `$`, `é`, `\pL`, `(a|b)`, `(?s).`, `(?m)^`, `(?m)$`, `\n`}
	suffixes := []string{``, ``, ``, `*`, `+`, `?`, `{2}`, `*?`, `+?`}
	rng := rand.New(rand.NewSource(1))
	for range 400 {
		var b strings.Builder
		for range 1 + rng.Intn(4) {
			b.WriteString(atoms[rng.Intn(len(atoms))] + suffixes[rng.Intn(len(suffixes))])
			if rng.Intn(8) == 0 {
				b.WriteString("|")
			}
		}
		patterns = append(patterns, b.String())
	}
	return patterns
}

// The same pattern must give the same answer as Go's own regular expressions, which is what
// the SQLite backend runs, on a UTF8 database.
func TestTranslatedRegexMatchesGo(t *testing.T) {
	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		t.Skip("PGSQL_TEST is required")
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(ctx) })

	var encoding string
	require.NoError(t, conn.QueryRow(ctx, "SELECT getdatabaseencoding()").Scan(&encoding))
	require.Equal(t, "UTF8", encoding)

	checked := 0
	for _, pattern := range regexTestPatterns() {
		re, err := regexp.Compile(pattern)
		require.NoError(t, err, pattern)
		translated, err := translateRegex(pattern)
		require.NoError(t, err, pattern)

		rows, err := conn.Query(ctx, "SELECT s, s ~ $1 FROM unnest($2::text[]) AS s", translated, regexTestStrings)
		require.NoError(t, err, pattern)
		count := 0
		for rows.Next() {
			var s string
			var got bool
			require.NoError(t, rows.Scan(&s, &got))
			assert.Equalf(t, re.MatchString(s), got, "pattern %q (%q), string %q", pattern, translated, s)
			count++
		}
		require.NoError(t, rows.Err(), "pattern %q (%q)", pattern, translated)
		require.Equal(t, len(regexTestStrings), count, pattern)
		checked += count
	}
	t.Logf("%d checks", checked)
}

// Raw SQL through querySQL and execSQL reaches regex_match without being translated.
func TestRegexMatchFromRawSQL(t *testing.T) {
	dsn := migrationFixture(t)
	_ = config.InitializeEmpty()

	ctx := context.Background()
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, dsn))
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	// each in its own transaction, the same as querySQL and execSQL
	query := func(sql string, args ...interface{}) (rows [][]interface{}, err error) {
		err = txn.WithTxn(ctx, db, func(ctx context.Context) error {
			var err error
			_, rows, err = db.QuerySQL(ctx, sql, args)
			return err
		})
		return rows, err
	}

	rows, err := query("SELECT regex_match('foo.mp4', '\\.mp4$'), regex_match('foo.mkv', '\\.mp4$')")
	require.NoError(t, err)
	assert.Equal(t, [][]interface{}{{true, false}}, rows)

	// NULL doesn't match, as with the earlier version of this function
	rows, err = query("SELECT regex_match(NULL, 'a')")
	require.NoError(t, err)
	assert.Equal(t, [][]interface{}{{false}}, rows)

	// patterns that only Go's syntax accepts, as a plugin written against SQLite might send
	for _, pattern := range []string{`foo(?i)bar`, `(?P<y>\d+)`, `\pL+`, `a{1000}`, `$*`} {
		_, err := query("SELECT regex_match('foo', ?)", pattern)
		require.Error(t, err, pattern)
		assert.Contains(t, err.Error(), pattern, "the error names the pattern")
		assert.Contains(t, err.Error(), "PostgreSQL's regular expression syntax", pattern)
		assert.Contains(t, err.Error(), "SQLSTATE 2201B", "keeps the invalid regex error code")
	}

	// written directly in the SQL, a ? in the pattern stays part of the pattern
	rows, err = query("SELECT regex_match('FOO', '(?i)foo'), regex_match('fo', 'fox?$')")
	require.NoError(t, err)
	assert.Equal(t, [][]interface{}{{true, true}}, rows)

	_, err = query("SELECT regex_match('a', '(?P<y>a)')")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PostgreSQL's regular expression syntax")

	// and alongside a real placeholder
	rows, err = query("SELECT regex_match(?, '(?i)foo')", "FOO")
	require.NoError(t, err)
	assert.Equal(t, [][]interface{}{{true}}, rows)

	// a percent sign in the pattern can't disturb the message
	_, err = query("SELECT regex_match('foo', ?)", `%s(`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `%s(`)

	// the connection still works, and the error doesn't reach other statements
	rows, err = query("SELECT regex_match('foo', 'f')")
	require.NoError(t, err)
	assert.Equal(t, [][]interface{}{{true}}, rows)

	// the same message through execSQL
	err = txn.WithTxn(ctx, db, func(ctx context.Context) error {
		_, _, err := db.ExecSQL(ctx, "SELECT regex_match('foo', ?)", []interface{}{`(?P<y>a)`})
		return err
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PostgreSQL's regular expression syntax")
}

func TestOpenRefusesOtherEncodings(t *testing.T) {
	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		t.Skip("PGSQL_TEST is required")
	}

	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close(ctx) })

	const name = "stash_test_latin1"
	_, err = admin.Exec(ctx, "CREATE DATABASE "+name+" ENCODING 'LATIN1' LOCALE 'C' TEMPLATE template0")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)") })

	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/" + name

	err = NewDatabase().Open(ctx, u.String())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LATIN1")
	assert.Contains(t, err.Error(), "UTF8")
}
