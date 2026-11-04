-- The version of Stash that last opened this database, and the one before it,
-- so a migration can tell apart databases from releases with the same schema
-- version. Stash writes it each time it opens the database at the current
-- schema version (pkg/database/app_version.go).
CREATE TABLE app_version (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  version TEXT NOT NULL,
  githash TEXT NOT NULL,
  opened_at timestamptz NOT NULL,
  previous_version TEXT,
  previous_githash TEXT,
  previous_opened_at timestamptz
);
