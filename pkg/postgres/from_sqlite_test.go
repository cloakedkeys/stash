//go:build integration

package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

// newSQLiteLibrary creates a current, empty SQLite library, and a raw connection for filling it.
func newSQLiteLibrary(t *testing.T) (string, *sqlx.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")

	db := sqlite.NewDatabase()
	require.NoError(t, db.Open(context.Background(), path))
	require.NoError(t, db.Close())

	raw, err := sqlx.Open("sqlite3", "file:"+filepath.ToSlash(path))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	return path, raw
}

const sqliteLibraryRows = `
INSERT INTO folders (id, path, basename, parent_folder_id, mod_time, created_at, updated_at) VALUES
  (101, '/data', 'data', NULL, '2024-01-02T03:04:05+01:00', '2024-01-02T03:04:05Z', '2024-01-02 03:04:05.5+00:00'),
  (102, '/unix', 'unix', NULL, 1700000000, 1700000000000, '2024-01-02T03:04:05Z');
INSERT INTO files (id, basename, zip_file_id, parent_folder_id, size, mod_time, created_at, updated_at) VALUES
  (7, '日本語 é.mp4', NULL, 101, 5000000000, '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z');
INSERT INTO files_fingerprints (file_id, type, fingerprint) VALUES
  (7, 'phash', -1234567890123), (7, 'oshash', 'abc123');
INSERT INTO scenes (id, title, date, date_precision, organized, created_at, updated_at) VALUES
  (5, 'A title', '2020-02-29', 0, 1, '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z'),
  (6, 'No date', NULL, NULL, 0, '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z'),
  (900, 'Blank date', '', NULL, 0, '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z');
INSERT INTO scene_custom_fields (scene_id, field, value) VALUES
  (5, 'text', 'hello "world"'), (5, 'number', 42), (5, 'decimal', 1.5), (5, 'looks like a number', '42'), (5, 'big', 9007199254740993);
INSERT INTO groups (id, name, organized, created_at, updated_at) VALUES
  (11, 'Organized group', 1, '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z'),
  (12, 'Unorganized group', 0, '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z');
INSERT INTO groups (id, name, created_at, updated_at) VALUES
  (13, 'Default group', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z');
INSERT INTO tags (id, name, created_at, updated_at) VALUES (3, 'A tag', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z');
INSERT INTO scenes_tags (scene_id, tag_id) VALUES (5, 3);
INSERT INTO blobs (checksum, blob) VALUES ('sum', X'00FF10');
INSERT INTO saved_filters (id, user_id, mode, name, find_filter, object_filter, ui_options) VALUES
  (2, 9, 'SCENES', 'Saved', '{"q":"x"}', '', NULL);
INSERT INTO voice_actors (id, name, details, aliases, urls, created_at, updated_at) VALUES
  (4, 'Voice', '', '["one"]', '[]', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z');
`

func TestMigrateFromSQLite(t *testing.T) {
	dsn := migrationFixture(t)
	_ = config.InitializeEmpty()
	ctx := context.Background()

	path, raw := newSQLiteLibrary(t)
	_, err := raw.Exec(sqliteLibraryRows)
	require.NoError(t, err)

	// a deleted row, so that its ID must not be used again
	_, err = raw.Exec("DELETE FROM scenes WHERE id = 900")
	require.NoError(t, err)
	_, err = raw.Exec("DELETE FROM users")
	require.NoError(t, err)
	_, err = raw.Exec(`INSERT INTO users (id, username, locked, password_hash, created_at, updated_at) VALUES
		(9, 'someone', 0, 'a hash', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z')`)
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO user_roles (user_id, role) VALUES (9, 'ADMIN')")
	require.NoError(t, err)

	// Personal favorites are copied for every supported type and user.
	_, err = raw.Exec(`INSERT INTO users (id, username, locked, created_at, updated_at)
 VALUES (10, 'other', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	for _, table := range []string{"images", "galleries", "groups", "audios"} {
		_, err = raw.Exec("INSERT INTO " + table + " (id, created_at, updated_at) VALUES (5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)")
		require.NoError(t, err)
	}
	for table, idColumn := range map[string]string{"scenes": "scene_id", "images": "image_id", "galleries": "gallery_id", "groups": "group_id", "audios": "audio_id"} {
		_, err = raw.Exec("INSERT INTO " + table + "_user_data (" + idColumn + ", user_id, favorite) VALUES (5, 9, 1), (5, 10, 0)")
		require.NoError(t, err)
	}

	before, err := os.ReadFile(path)
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, MigrateFromSQLite(ctx, path, dsn, &out))
	assert.Contains(t, out.String(), "Copied scenes: 2 rows")
	assert.Contains(t, out.String(), "Finished")

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(before, after), "the SQLite file isn't changed")

	db := NewDatabase()
	require.NoError(t, db.Open(ctx, dsn))
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	get := func(dest any, q string, args ...any) {
		t.Helper()
		require.NoError(t, db.readDB.Get(dest, q, args...), q)
	}

	var count int
	get(&count, "SELECT COUNT(*) FROM scenes")
	assert.Equal(t, 2, count)

	// IDs are kept, and the deleted one isn't reused
	var nextScene int
	require.NoError(t, db.writeDB.Get(&nextScene, "INSERT INTO scenes (title, created_at, updated_at) VALUES ('new', now(), now()) RETURNING id"))
	assert.Equal(t, 901, nextScene)
	_, err = db.writeDB.Exec("DELETE FROM scenes WHERE id = $1", nextScene)
	require.NoError(t, err)

	// a table that was empty still numbers from 1
	var nextMarker int
	require.NoError(t, db.writeDB.Get(&nextMarker, "SELECT nextval(pg_get_serial_sequence('scene_markers', 'id'))"))
	assert.Equal(t, 1, nextMarker)

	// types
	var size int64
	get(&size, "SELECT size FROM files WHERE id = 7")
	assert.Equal(t, int64(5000000000), size)

	var name string
	get(&name, "SELECT basename FROM files WHERE id = 7")
	assert.Equal(t, "日本語 é.mp4", name)

	var date string
	get(&date, "SELECT date::text FROM scenes WHERE id = 5")
	assert.Equal(t, "2020-02-29", date)
	var noDate sql.NullString
	get(&noDate, "SELECT date::text FROM scenes WHERE id = 6")
	assert.False(t, noDate.Valid)

	var organized bool
	get(&organized, "SELECT organized FROM scenes WHERE id = 5")
	assert.True(t, organized)

	get(&organized, "SELECT organized FROM groups WHERE id = 11")
	assert.True(t, organized)
	for _, id := range []int{12, 13} {
		get(&organized, "SELECT organized FROM groups WHERE id = $1", id)
		assert.False(t, organized)
	}

	var created time.Time
	get(&created, "SELECT mod_time FROM folders WHERE id = 101")
	assert.True(t, created.Equal(time.Date(2024, 1, 2, 2, 4, 5, 0, time.UTC)), "keeps the time zone offset: %s", created)

	// numbers in a date column are read the way the SQLite driver reads them
	get(&created, "SELECT mod_time FROM folders WHERE id = 102")
	assert.True(t, created.Equal(time.Unix(1700000000, 0)), "seconds: %s", created)
	get(&created, "SELECT created_at FROM folders WHERE id = 102")
	assert.True(t, created.Equal(time.Unix(1700000000, 0)), "milliseconds: %s", created)

	var blob []byte
	get(&blob, "SELECT blob FROM blobs WHERE checksum = 'sum'")
	assert.Equal(t, []byte{0x00, 0xFF, 0x10}, blob)

	var aliases string
	get(&aliases, "SELECT aliases::text FROM voice_actors WHERE id = 4")
	assert.JSONEq(t, `["one"]`, aliases)

	var emptyFilter sql.NullString
	get(&emptyFilter, "SELECT object_filter::text FROM saved_filters WHERE id = 2")
	assert.False(t, emptyFilter.Valid, "an empty filter becomes no filter")

	// the starter account is replaced by the library's accounts
	var usernames []string
	require.NoError(t, db.readDB.Select(&usernames, "SELECT username FROM users"))
	assert.ElementsMatch(t, []string{"someone", "other"}, usernames)
	for _, table := range []string{"scenes", "images", "galleries", "groups", "audios"} {
		var favorite bool
		get(&favorite, "SELECT favorite FROM "+table+"_user_data WHERE user_id = 9")
		assert.True(t, favorite, table)
		get(&favorite, "SELECT favorite FROM "+table+"_user_data WHERE user_id = 10")
		assert.False(t, favorite, table)
	}

	// through Stash's own code
	require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
		fields, err := db.Scene().GetCustomFields(ctx, 5)
		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{
			"text":                `hello "world"`,
			"number":              int64(42),
			"decimal":             1.5,
			"looks like a number": "42",
			"big":                 int64(9007199254740993),
		}, fields)

		f, err := db.File().Find(ctx, models.FileID(7))
		require.NoError(t, err)
		require.Len(t, f, 1)
		assert.Equal(t, int64(-1234567890123), f[0].Base().Fingerprints.Get(models.FingerprintTypePhash))
		assert.Equal(t, "abc123", f[0].Base().Fingerprints.Get(models.FingerprintTypeOshash))
		return nil
	}))

	// a second run can't overwrite anything
	out.Reset()
	err = MigrateFromSQLite(ctx, path, dsn, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has data")
	get(&count, "SELECT COUNT(*) FROM scenes")
	assert.Equal(t, 2, count)
}

func TestMigrateFromSQLiteReportsBadValues(t *testing.T) {
	dsn := migrationFixture(t)
	_ = config.InitializeEmpty()
	ctx := context.Background()

	path, raw := newSQLiteLibrary(t)
	_, err := raw.Exec(`
		INSERT INTO folders (id, path, basename, mod_time, created_at, updated_at) VALUES
		  (101, CAST(X'2F6461FF7461' AS TEXT), 'data', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z'),
		  (102, '/fine', 'fine', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z');
		INSERT INTO scenes (id, title, date, created_at, updated_at) VALUES
		  (5, 'Bad date', '2020-13-45', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z');
	`)
	require.NoError(t, err)

	var out bytes.Buffer
	err = MigrateFromSQLite(ctx, path, dsn, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing was copied")
	assert.Contains(t, err.Error(), "folders, row with id = 101, column path")
	assert.Contains(t, err.Error(), "scenes, row with id = 5, column date")
	assert.Contains(t, err.Error(), "2020-13-45")

	check, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, check.Close()) })

	var count int
	require.NoError(t, check.Get(&count, "SELECT COUNT(*) FROM folders WHERE id > 100"))
	assert.Zero(t, count, "the good rows aren't kept either")
	require.NoError(t, check.Get(&count, "SELECT COUNT(*) FROM users"))
	assert.Equal(t, 1, count, "the starter account is still there")

	// after fixing the values, it can run again
	_, err = raw.Exec("UPDATE folders SET path = '/data' WHERE id = 101; UPDATE scenes SET date = '2020-12-25' WHERE id = 5")
	require.NoError(t, err)
	require.NoError(t, MigrateFromSQLite(ctx, path, dsn, &out))
	require.NoError(t, check.Get(&count, "SELECT COUNT(*) FROM folders WHERE id > 100"))
	assert.Equal(t, 2, count)
}

// What only PostgreSQL's own rules catch still can't hide the problems already found.
func TestMigrateFromSQLiteReportsRequiredValues(t *testing.T) {
	dsn := migrationFixture(t)
	_ = config.InitializeEmpty()
	ctx := context.Background()

	target := NewDatabase()
	require.NoError(t, target.Open(ctx, dsn))
	_, err := target.writeDB.Exec("ALTER TABLE tags ADD CONSTRAINT zz_short CHECK (length(name) < 3)")
	require.NoError(t, err)
	_, err = target.writeDB.Exec("ALTER TABLE performers ALTER COLUMN details SET NOT NULL")
	require.NoError(t, err)
	require.NoError(t, target.Close())

	path, raw := newSQLiteLibrary(t)
	_, err = raw.Exec(`
		INSERT INTO performers (id, name, created_at, updated_at) VALUES (8, 'No details', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z');
		INSERT INTO scenes (id, title, date, created_at, updated_at) VALUES (5, 'Bad date', '2020-13-45', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z');
		INSERT INTO tags (id, name, created_at, updated_at) VALUES (3, 'A long tag', '2024-01-02T03:04:05Z', '2024-01-02T03:04:05Z');
	`)
	require.NoError(t, err)

	var out bytes.Buffer
	err = MigrateFromSQLite(ctx, path, dsn, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "copying tags")
	assert.Contains(t, err.Error(), "zz_short")
	assert.Contains(t, err.Error(), "performers, row with id = 8, column details")
	assert.Contains(t, err.Error(), "scenes, row with id = 5, column date")
}

func TestMigrateFromSQLiteRefuses(t *testing.T) {
	_ = config.InitializeEmpty()
	ctx := context.Background()
	var out bytes.Buffer

	t.Run("missing file", func(t *testing.T) {
		dsn := migrationFixture(t)
		err := MigrateFromSQLite(ctx, filepath.Join(t.TempDir(), "none.sqlite"), dsn, &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading SQLite file")
	})

	t.Run("not a Stash database", func(t *testing.T) {
		dsn := migrationFixture(t)
		path := filepath.Join(t.TempDir(), "other.sqlite")
		require.NoError(t, os.WriteFile(path, []byte("not a database"), 0o600))
		err := MigrateFromSQLite(ctx, path, dsn, &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "can't read this as a Stash SQLite database")
	})

	for name, c := range map[string]struct {
		q    string
		want string
	}{
		"older":      {"UPDATE schema_migrations SET version = version - 1", "Start this build of Stash on the SQLite database"},
		"newer":      {"UPDATE schema_migrations SET version = version + 1", "newer than this build"},
		"unfinished": {"UPDATE schema_migrations SET dirty = 1", "didn't finish"},
	} {
		t.Run(name, func(t *testing.T) {
			dsn := migrationFixture(t)
			path, raw := newSQLiteLibrary(t)
			_, err := raw.Exec(c.q)
			require.NoError(t, err)
			err = MigrateFromSQLite(ctx, path, dsn, &out)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
		})
	}

	for name, c := range map[string]struct {
		q    string
		want string
	}{
		"older PostgreSQL database":      {"UPDATE schema_migrations SET version = version - 1", "isn't at the version this build needs"},
		"unfinished PostgreSQL database": {"UPDATE schema_migrations SET dirty = true", "didn't finish"},
	} {
		t.Run(name, func(t *testing.T) {
			dsn := migrationFixture(t)
			target := NewDatabase()
			require.NoError(t, target.Open(ctx, dsn))
			_, err := target.writeDB.Exec(c.q)
			require.NoError(t, err)
			require.NoError(t, target.Close())

			path, _ := newSQLiteLibrary(t)
			err = MigrateFromSQLite(ctx, path, dsn, &out)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
		})
	}

	t.Run("a table only a plugin knows is reported", func(t *testing.T) {
		dsn := migrationFixture(t)
		path, raw := newSQLiteLibrary(t)
		_, err := raw.Exec("CREATE TABLE plugin_table (id INTEGER); INSERT INTO plugin_table VALUES (1)")
		require.NoError(t, err)
		out.Reset()
		require.NoError(t, MigrateFromSQLite(ctx, path, dsn, &out))
		assert.Contains(t, out.String(), "WARNING: not copying plugin_table (1 rows)")
	})

	t.Run("a new column in SQLite stops it, rather than losing the data", func(t *testing.T) {
		dsn := migrationFixture(t)
		path, raw := newSQLiteLibrary(t)
		_, err := raw.Exec("ALTER TABLE tags ADD COLUMN extra TEXT")
		require.NoError(t, err)
		err = MigrateFromSQLite(ctx, path, dsn, &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tags.extra")
	})
}

// A new table or column on one database and not the other must fail here, rather than in someone's
// migration, so that the copy always keeps up.
func TestMigrateFromSQLiteCoversEveryTable(t *testing.T) {
	dsn := migrationFixture(t)
	_ = config.InitializeEmpty()

	path, _ := newSQLiteLibrary(t)
	var out bytes.Buffer
	require.NoError(t, MigrateFromSQLite(context.Background(), path, dsn, &out))
	assert.NotContains(t, out.String(), "WARNING", "every SQLite table is either copied or known to be old")
}
