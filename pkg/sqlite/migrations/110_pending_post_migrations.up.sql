-- #2221: the schema versions whose follow-up Go step (post migration) hasn't
-- finished. A migration that has one adds its row in the same transaction as
-- its SQL, and the row is removed once the step has finished, so a step that
-- was cut off, or failed without the backup being put back, runs again on the
-- next start, before the database is used (pkg/sqlite/migrate.go). Migrations
-- before this one create the table themselves when they need it, so it may
-- already exist.
CREATE TABLE IF NOT EXISTS `pending_post_migrations` (
  `version` INTEGER PRIMARY KEY
);
