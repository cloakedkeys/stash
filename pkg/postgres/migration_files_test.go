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
