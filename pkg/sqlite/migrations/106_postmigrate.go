package migrations

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/savedfilter"
	"github.com/stashapp/stash/pkg/sqlite"
)

// post106 rewrites saved filters and default filters that used _ or % as
// wildcards in text filters, which now match only themselves, so they find
// the same things as before.
func post106(ctx context.Context, db *sqlx.DB) error {
	logger.Info("Running post-migration for schema version 106")

	m := schema106Migrator{
		migrator: migrator{
			db: db,
		},
	}

	// not withTxn, which doesn't return a failed commit
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := m.migrateSavedFilters(tx); err != nil {
		return fmt.Errorf("rewriting saved filters: %w", err)
	}
	if err := m.migrateDefaultFilters(tx); err != nil {
		return fmt.Errorf("rewriting default filters: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	// after the database is migrated, so a config file that can't be written
	// never leaves it half done; this only logs failures
	savedfilter.ConvertLegacyWildcardConfigDefaultFilters(ctx, savedfilter.LegacySQLite, config.GetInstance(), "105")
	return nil
}

type schema106Migrator struct {
	migrator
}

type schema106SavedFilter struct {
	ID           int    `db:"id"`
	Name         string `db:"name"`
	Mode         string `db:"mode"`
	FindFilter   string `db:"find_filter"`
	ObjectFilter string `db:"object_filter"`
}

func (m *schema106Migrator) migrateSavedFilters(tx *sqlx.Tx) error {
	var rows []schema106SavedFilter
	if err := tx.Select(&rows, "SELECT `id`, `name`, `mode`, COALESCE(`find_filter`, '') AS `find_filter`, COALESCE(`object_filter`, '') AS `object_filter` FROM `saved_filters`"); err != nil {
		return err
	}

	for _, r := range rows {
		desc := fmt.Sprintf("saved filter %d (%q)", r.ID, r.Name)

		if note := savedfilter.LegacyWildcardSearchNote(r.FindFilter); note != "" {
			logger.Warnf("Left %s as it was: %s", desc, note)
		}

		newFilter, changed, notes, err := savedfilter.ConvertLegacyWildcardsJSON(savedfilter.LegacySQLite, models.FilterMode(r.Mode), r.ObjectFilter)
		if err != nil {
			logger.Warnf("Left %s as it was: can't read its filter: %v", desc, err)
			continue
		}
		for _, note := range notes {
			logger.Warnf("Left part of %s as it was: %s", desc, note)
		}
		if !changed {
			continue
		}

		if _, err := tx.Exec("UPDATE `saved_filters` SET `object_filter` = ? WHERE `id` = ?", newFilter, r.ID); err != nil {
			return fmt.Errorf("updating %s: %w", desc, err)
		}
		logger.Infof("Rewrote the text filters in %s that used _ or %% as wildcards as regular expressions", desc)
	}

	return nil
}

type schema106UIConfig struct {
	UserID int    `db:"user_id"`
	Config string `db:"config"`
}

func (m *schema106Migrator) migrateDefaultFilters(tx *sqlx.Tx) error {
	var rows []schema106UIConfig
	if err := tx.Select(&rows, "SELECT `user_id`, `config` FROM `user_ui_config`"); err != nil {
		return err
	}

	for _, r := range rows {
		desc := fmt.Sprintf("user %d's default filters", r.UserID)

		newConfig, changed, notes, err := savedfilter.ConvertLegacyWildcardDefaultFilters(savedfilter.LegacySQLite, r.Config)
		if err != nil {
			logger.Warnf("Left %s as they were: can't read the UI configuration: %v", desc, err)
			continue
		}
		for _, note := range notes {
			logger.Warnf("Left part of %s as it was: %s", desc, note)
		}
		if !changed {
			continue
		}

		if _, err := tx.Exec("UPDATE `user_ui_config` SET `config` = ? WHERE `user_id` = ?", newConfig, r.UserID); err != nil {
			return fmt.Errorf("updating %s: %w", desc, err)
		}
		logger.Infof("Rewrote the text filters in %s that used _ or %% as wildcards as regular expressions", desc)
	}

	return nil
}

func init() {
	sqlite.RegisterPostMigration(106, post106)
}
