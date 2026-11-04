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
// version used (#2057). It has one row, written each time the database is
// opened at the current schema version. When a different version opens it, the
// one before is kept in the previous_ columns.
//
// Copying a SQLite library to PostgreSQL leaves it out: each database records
// what opened it.
const AppVersionTable = "app_version"

// AppVersion is a version of Stash, as internal/build gives it.
type AppVersion struct {
	Version string
	GitHash string
}

// CurrentAppVersion is the version of this build.
func CurrentAppVersion() AppVersion {
	version, githash, _ := build.Version()
	return AppVersion{Version: version, GitHash: githash}
}

// StampAppVersion records v as the version that opened the database at now.
// If another version opened it last, that one becomes the previous version.
// The same SQL runs on SQLite and PostgreSQL.
func StampAppVersion(ctx context.Context, db *sqlx.DB, v AppVersion, now time.Time) error {
	const same = AppVersionTable + ".version = excluded.version AND " + AppVersionTable + ".githash = excluded.githash"
	q := `INSERT INTO ` + AppVersionTable + ` (id, version, githash, opened_at) VALUES (1, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
	previous_version = CASE WHEN ` + same + ` THEN ` + AppVersionTable + `.previous_version ELSE ` + AppVersionTable + `.version END,
	previous_githash = CASE WHEN ` + same + ` THEN ` + AppVersionTable + `.previous_githash ELSE ` + AppVersionTable + `.githash END,
	previous_opened_at = CASE WHEN ` + same + ` THEN ` + AppVersionTable + `.previous_opened_at ELSE ` + AppVersionTable + `.opened_at END,
	version = excluded.version,
	githash = excluded.githash,
	opened_at = excluded.opened_at`
	if _, err := db.ExecContext(ctx, db.Rebind(q), v.Version, v.GitHash, now.UTC()); err != nil {
		return fmt.Errorf("recording the Stash version in %s: %w", AppVersionTable, err)
	}
	return nil
}

// PreviousAppVersion is for post-migrations. It returns the version of Stash
// that last opened the database before this upgrade, such as "v0.33.0", or ""
// when that isn't known: a database is only stamped once it's at the current
// schema version, so during a migration the stamp is still the old build's.
// Every database from before the app_version table, and a new one, gives "".
// db is the connection the post-migration was given.
func PreviousAppVersion(ctx context.Context, db *sqlx.DB) (string, error) {
	var version string
	err := db.GetContext(ctx, &version, "SELECT version FROM "+AppVersionTable+" WHERE id = 1")
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading the Stash version from %s: %w", AppVersionTable, err)
	}
	return version, nil
}
