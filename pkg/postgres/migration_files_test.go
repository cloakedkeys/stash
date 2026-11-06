package postgres

import (
	"io/fs"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A failed migration keeps none of its changes only while each file runs as
// one transaction, and a schema version left unfinished is set back on that
// basis (resetFailedMigration). A file with its own transaction control would
// break that, so none may have any. BEGIN inside a function body has no
// semicolon after it and isn't matched.
func TestMigrationFilesHaveNoTransactionControl(t *testing.T) {
	control := regexp.MustCompile(`(?im)^\s*(?:(?:COMMIT|ROLLBACK|ABORT|SAVEPOINT|RELEASE|START\s+TRANSACTION|BEGIN\s+(?:TRANSACTION|WORK)|END\s+(?:TRANSACTION|WORK))\b|BEGIN\s*;)`)
	for _, s := range []string{"BEGIN;", "commit;", "  START TRANSACTION;", "x;\nCOMMIT"} {
		assert.Truef(t, control.MatchString(s), "should match %q", s)
	}
	for _, s := range []string{"BEGIN\n  RETURN 1;", "END;", "committed_at INTEGER"} {
		assert.Falsef(t, control.MatchString(s), "should not match %q", s)
	}

	files, err := fs.Glob(migrationsBox, "migrations/*.up.sql")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		body, err := fs.ReadFile(migrationsBox, file)
		require.NoError(t, err)
		assert.Emptyf(t, control.FindAllString(string(body), -1), "%s has its own transaction control", file)
	}
}

// Statements that PostgreSQL refuses to run inside a transaction would make a
// migration file fail every time, since the file runs as one, so none may
// have any (#2202). ALTER TYPE ... ADD VALUE does run in one, but the new
// value can't be used before it commits, so it is refused too. Only --
// comments are left out, so they can mention them; /* */ comments aren't.
func TestMigrationFilesHaveNoStatementsOutsideTransactions(t *testing.T) {
	comment := regexp.MustCompile(`--[^\n]*`)
	outside := regexp.MustCompile(`(?is)\b(?:CONCURRENTLY|VACUUM|ALTER\s+SYSTEM|(?:CREATE|DROP|ALTER)\s+DATABASE|(?:CREATE|DROP)\s+TABLESPACE|REINDEX\s+(?:DATABASE|SYSTEM)|DISCARD\s+ALL|ALTER\s+TYPE\s+\S+\s+ADD\s+VALUE)\b`)
	for _, s := range []string{"CREATE INDEX CONCURRENTLY i ON t (a);", "vacuum analyze t;", "ALTER TYPE mood ADD VALUE 'ok';", "DROP DATABASE x;", "ALTER SYSTEM SET x = 1;"} {
		assert.Truef(t, outside.MatchString(s), "should match %q", s)
	}
	for _, s := range []string{"CREATE INDEX i ON t (a);", "ALTER TABLE t ADD COLUMN vacuumed BOOLEAN;", "ALTER TYPE mood RENAME VALUE 'a' TO 'b';"} {
		assert.Falsef(t, outside.MatchString(s), "should not match %q", s)
	}

	files, err := fs.Glob(migrationsBox, "migrations/*.up.sql")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		body, err := fs.ReadFile(migrationsBox, file)
		require.NoError(t, err)
		code := comment.ReplaceAllString(string(body), "")
		assert.Emptyf(t, outside.FindAllString(code, -1), "%s has a statement that can't run inside a transaction", file)
	}
}
