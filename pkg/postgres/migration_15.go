package postgres

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/user"
)

// The SQL migration reserves a locked admin so foreign keys can be populated
// before legacy credentials are read. A failed post-migration leaves it locked.
func post15(ctx context.Context, db *sqlx.DB) error {
	if models.IsSetupContext(ctx) {
		result, err := db.ExecContext(ctx, `DELETE FROM users WHERE id=1 AND locked
AND NOT EXISTS (SELECT 1 FROM scenes_user_data)
AND NOT EXISTS (SELECT 1 FROM images_user_data)
AND NOT EXISTS (SELECT 1 FROM galleries_user_data)
AND NOT EXISTS (SELECT 1 FROM groups_user_data)
AND NOT EXISTS (SELECT 1 FROM performers_user_data)
AND NOT EXISTS (SELECT 1 FROM studios_user_data)
AND NOT EXISTS (SELECT 1 FROM tags_user_data)
AND NOT EXISTS (SELECT 1 FROM voice_actors_user_data)
AND NOT EXISTS (SELECT 1 FROM saved_filters)
AND NOT EXISTS (SELECT 1 FROM scenes_o_dates)
AND NOT EXISTS (SELECT 1 FROM scenes_view_dates)`)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("cannot initialize a new account over existing personal data")
		}
		_, err = db.ExecContext(ctx, "SELECT setval(pg_get_serial_sequence('users','id'),1,false)")
		return err
	}
	cfg := config.GetInstance()
	username, password, apiKey := cfg.GetLegacyUsername(), cfg.GetLegacyPasswordHash(), cfg.GetLegacyAPIKey()
	legacy := username != "" && password != ""
	if username == "" {
		username = "admin"
	}
	var err error
	if legacy {
		password, err = user.EncodeLegacyHash(password)
		if err != nil {
			return err
		}
	} else {
		password = ""
	}
	if apiKey != "" {
		apiKey, err = user.HashAPIKey(apiKey)
		if err != nil {
			return err
		}
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET username=$1,password_hash=NULLIF($2,''),api_key=NULLIF($3,''),locked=false,updated_at=CURRENT_TIMESTAMP WHERE id=1`, username, password, apiKey); err != nil {
		return err
	}
	if legacy {
		cfg.SetPublicAccess(true)
	}
	if cfg.GetConfigFile() != "" && (cfg.GetLegacyUsername() != "" || cfg.GetLegacyPasswordHash() != "" || cfg.GetLegacyAPIKey() != "") {
		data, err := cfg.Marshal()
		if err != nil {
			return err
		}
		backup := fmt.Sprintf("%s.pg14.%s", cfg.GetConfigFile(), time.Now().Format("20060102_150405"))
		if err := os.WriteFile(backup, data, 0644); err != nil {
			return err
		}
		cfg.SetInterface(config.LegacyUsername, nil)
		cfg.SetInterface(config.LegacyPassword, nil)
		cfg.SetInterface(config.LegacyAPIKey, nil)
		if err := cfg.Write(); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func init() { RegisterPostMigration(15, post15) }
