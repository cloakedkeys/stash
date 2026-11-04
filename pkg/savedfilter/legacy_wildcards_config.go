package savedfilter

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

// ConvertLegacyWildcardConfigDefaultFilters rewrites the default filters in the
// config file's UI configuration, as ConvertLegacyWildcardDefaultFiltersMap
// does. Users without a UI configuration of their own use these, and they're
// the only default filters of a database that came from upstream Stash.
//
// The config file belongs to the server rather than to one database, so this
// runs once per config file: it sets config.LegacyWildcardsConverted in the
// same write, even when nothing needed rewriting, and does nothing once that's
// set. Otherwise creating a new database, or copying one from SQLite to
// PostgreSQL, would rewrite default filters this build saved.
//
// The migrations call this after their database transaction commits. A
// failure here is logged and doesn't fail the migration: the database is
// already migrated, and the config file is left as it was, so it's never half
// written. backupTag names the backup, as in config.yml.<backupTag>.<time>.
func ConvertLegacyWildcardConfigDefaultFilters(ctx context.Context, dialect LegacyDialect, cfg *config.Config, backupTag string) {
	// a new setup has no default filters yet, and its new config file has
	// the marker
	if models.IsSetupContext(ctx) || cfg == nil {
		return
	}
	configFile := cfg.GetConfigFile()
	if configFile == "" {
		// no config file loaded, as in tests
		return
	}
	if cfg.GetLegacyWildcardsConverted() {
		return
	}

	const desc = "the default filters in the config file"
	uiConfig := cfg.GetUIConfiguration()
	changed, notes := ConvertLegacyWildcardDefaultFiltersMap(dialect, uiConfig)
	for _, note := range notes {
		logger.Warnf("Left part of %s as it was: %s", desc, note)
	}
	if !changed {
		cfg.SetBool(config.LegacyWildcardsConverted, true)
		if err := cfg.Write(); err != nil {
			cfg.SetInterface(config.LegacyWildcardsConverted, nil)
			logger.Warnf("Can't mark %s as needing no rewrite: can't save the config file: %v", desc, err)
		}
		return
	}

	data, err := cfg.Marshal()
	if err != nil {
		logger.Warnf("Left %s as they were: can't back up the config file: %v", desc, err)
		return
	}
	backupPath := fmt.Sprintf("%s.%s.%s", configFile, backupTag, time.Now().Format("20060102_150405"))
	logger.Infof("Backing up config to %s", backupPath)
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		logger.Warnf("Left %s as they were: can't back up the config file: %v", desc, err)
		return
	}

	// GetUIConfiguration returns a copy, so this is still as it was
	original := cfg.GetUIConfiguration()
	cfg.SetUIConfiguration(uiConfig)
	cfg.SetBool(config.LegacyWildcardsConverted, true)
	if err := cfg.Write(); err != nil {
		cfg.SetUIConfiguration(original)
		cfg.SetInterface(config.LegacyWildcardsConverted, nil)
		logger.Warnf("Left %s as they were: can't save the config file: %v", desc, err)
		return
	}
	logger.Infof("Rewrote the text filters in %s that used _ or %% as wildcards as regular expressions", desc)
}
