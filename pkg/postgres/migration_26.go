package postgres

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/savedfilter"
)

// post26 rewrites saved filters and default filters that used _ or % as
// wildcards in text filters, which now match only themselves, so they find
// the same things as before.
func post26(ctx context.Context, db *sqlx.DB) error {
	logger.Info("Running post-migration for schema version 26")

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := migrate26SavedFilters(ctx, tx); err != nil {
		return fmt.Errorf("rewriting saved filters: %w", err)
	}
	if err := migrate26DefaultFilters(ctx, tx); err != nil {
		return fmt.Errorf("rewriting default filters: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// after the database is migrated, so a config file that can't be written
	// never leaves it half done; this only logs failures
	savedfilter.ConvertLegacyWildcardConfigDefaultFilters(ctx, savedfilter.LegacyPostgres, config.GetInstance(), "pg25")
	return nil
}

type migration26SavedFilter struct {
	ID           int    `db:"id"`
	Name         string `db:"name"`
	Mode         string `db:"mode"`
	FindFilter   string `db:"find_filter"`
	ObjectFilter string `db:"object_filter"`
}

func migrate26SavedFilters(ctx context.Context, tx *sqlx.Tx) error {
	var rows []migration26SavedFilter
	if err := tx.SelectContext(ctx, &rows, `SELECT id, name, mode, COALESCE(find_filter::text, '') AS find_filter, COALESCE(object_filter::text, '') AS object_filter FROM saved_filters`); err != nil {
		return err
	}

	for _, r := range rows {
		desc := fmt.Sprintf("saved filter %d (%q)", r.ID, r.Name)

		if note := savedfilter.LegacyWildcardSearchNote(r.FindFilter); note != "" {
			logger.Warnf("Left %s as it was: %s", desc, note)
		}

		newFilter, changed, notes, err := savedfilter.ConvertLegacyWildcardsJSON(savedfilter.LegacyPostgres, models.FilterMode(r.Mode), r.ObjectFilter)
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

		if _, err := tx.ExecContext(ctx, `UPDATE saved_filters SET object_filter = $1::jsonb WHERE id = $2`, newFilter, r.ID); err != nil {
			return fmt.Errorf("updating %s: %w", desc, err)
		}
		logger.Infof("Rewrote the text filters in %s that used _ or %% as wildcards as regular expressions", desc)
	}

	return nil
}

type migration26UIConfig struct {
	UserID int    `db:"user_id"`
	Config string `db:"config"`
}

func migrate26DefaultFilters(ctx context.Context, tx *sqlx.Tx) error {
	var rows []migration26UIConfig
	if err := tx.SelectContext(ctx, &rows, `SELECT user_id, config FROM user_ui_config`); err != nil {
		return err
	}

	for _, r := range rows {
		desc := fmt.Sprintf("user %d's default filters", r.UserID)

		newConfig, changed, notes, err := savedfilter.ConvertLegacyWildcardDefaultFilters(savedfilter.LegacyPostgres, r.Config)
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

		if _, err := tx.ExecContext(ctx, `UPDATE user_ui_config SET config = $1 WHERE user_id = $2`, newConfig, r.UserID); err != nil {
			return fmt.Errorf("updating %s: %w", desc, err)
		}
		logger.Infof("Rewrote the text filters in %s that used _ or %% as wildcards as regular expressions", desc)
	}

	return nil
}

func init() {
	RegisterPostMigration(26, post26)
}
