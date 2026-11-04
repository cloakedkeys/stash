package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/stashapp/stash/internal/build"
)

// AppVersionTable records which version of Stash last opened the database, so
// a migration can tell apart databases that two releases with the same schema
// version used (#2057). It has one row, written when a build opens the
// database at the current schema version and the row names another build.
// The build it names then moves to the previous_ columns. Opening it again
// with the same build writes nothing, so first_opened_at is when this build
// first opened it.
//
// Copying a SQLite library to PostgreSQL leaves it out: each database records
// what opened it.
const AppVersionTable = "app_version"

// AppVersionStampTimeout is the longest opening a database waits to record
// the version, such as when another connection holds the row or the write
// lock. Recording it is only worth a warning, not a slow start.
const AppVersionStampTimeout = 5 * time.Second

// AppVersionDB is what StampAppVersion needs: a pool, or one connection from it.
type AppVersionDB interface {
	sqlx.QueryerContext
	sqlx.ExecerContext
	Rebind(query string) string
}

// AppVersion is a version of Stash, as internal/build gives it.
type AppVersion struct {
	Version string `db:"version"`
	GitHash string `db:"githash"`
}

// CurrentAppVersion is the version of this build.
func CurrentAppVersion() AppVersion {
	version, githash, _ := build.Version()
	return AppVersion{Version: version, GitHash: githash}
}

// StampAppVersion records v as the version that opened the database, at now,
// unless the row already names v, in which case nothing is written. If it
// names another version, that one becomes the previous version. It reports
// whether it wrote. The same SQL runs on SQLite and PostgreSQL.
func StampAppVersion(ctx context.Context, db AppVersionDB, v AppVersion, now time.Time) (bool, error) {
	var stored []AppVersion
	if err := sqlx.SelectContext(ctx, db, &stored, "SELECT version, githash FROM "+AppVersionTable+" WHERE id = 1"); err != nil {
		return false, fmt.Errorf("reading the Stash version from %s: %w", AppVersionTable, err)
	}
	if len(stored) == 1 && stored[0] == v {
		return false, nil
	}

	const t = AppVersionTable
	q := `INSERT INTO ` + t + ` (id, version, githash, first_opened_at) VALUES (1, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
	previous_version = ` + t + `.version,
	previous_githash = ` + t + `.githash,
	previous_first_opened_at = ` + t + `.first_opened_at,
	version = excluded.version,
	githash = excluded.githash,
	first_opened_at = excluded.first_opened_at
WHERE ` + t + `.version <> excluded.version OR ` + t + `.githash <> excluded.githash`
	if _, err := db.ExecContext(ctx, db.Rebind(q), v.Version, v.GitHash, now.UTC()); err != nil {
		return false, fmt.Errorf("recording the Stash version in %s: %w", AppVersionTable, err)
	}
	return true, nil
}

// PreviousAppVersion is for post-migrations. It returns the version of Stash
// that last opened the database before this upgrade, such as "v0.33.0", or ""
// when that isn't known: a database is only stamped once it's at the current
// schema version, so during a migration the stamp is still the old build's.
// Every database from before the app_version table, and a new one, gives "".
//
// A build without a version, such as a plain go build, stamps "" too, and
// moves the release that opened the database before it into the previous_
// columns. A migration run after that sees "" rather than the release.
//
// db is the connection the post-migration was given.
func PreviousAppVersion(ctx context.Context, db sqlx.QueryerContext) (string, error) {
	var version string
	err := sqlx.GetContext(ctx, db, &version, "SELECT version FROM "+AppVersionTable+" WHERE id = 1")
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading the Stash version from %s: %w", AppVersionTable, err)
	}
	return version, nil
}
