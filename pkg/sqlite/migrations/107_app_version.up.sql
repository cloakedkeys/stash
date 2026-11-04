-- The version of Stash that last opened this database, and the one before it,
-- so a migration can tell apart databases from releases with the same schema
-- version. Stash writes it when it opens the database at the current schema
-- version and the row names another build, so first_opened_at is when the
-- build first opened it (pkg/database/app_version.go).
CREATE TABLE app_version (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  version TEXT NOT NULL,
  githash TEXT NOT NULL,
  first_opened_at DATETIME NOT NULL,
  previous_version TEXT,
  previous_githash TEXT,
  previous_first_opened_at DATETIME
);
