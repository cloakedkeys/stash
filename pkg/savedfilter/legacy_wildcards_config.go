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
// The migrations call this after their database transaction commits. A
// failure here is logged and doesn't fail the migration: the database is
// already migrated, and the config file is left as it was, so it's never half
// written. backupTag names the backup, as in config.yml.<backupTag>.<time>.
func ConvertLegacyWildcardConfigDefaultFilters(ctx context.Context, cfg *config.Config, backupTag string) {
	// a new setup has no default filters yet
	if models.IsSetupContext(ctx) || cfg == nil {
		return
	}
	configFile := cfg.GetConfigFile()
	if configFile == "" {
		// no config file loaded, as in tests
		return
	}

	uiConfig := cfg.GetUIConfiguration()
	if uiConfig == nil {
		return
	}

	const desc = "the default filters in the config file"
	changed, notes := ConvertLegacyWildcardDefaultFiltersMap(uiConfig)
	for _, note := range notes {
		logger.Warnf("Left part of %s as it was: %s", desc, note)
	}
	if !changed {
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
	if err := cfg.Write(); err != nil {
		cfg.SetUIConfiguration(original)
		logger.Warnf("Left %s as they were: can't save the config file: %v", desc, err)
		return
	}
	logger.Infof("Rewrote the text filters in %s that used _ or %% as wildcards as regular expressions", desc)
}
