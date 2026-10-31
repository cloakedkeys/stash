package migrations

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stashapp/stash/pkg/user"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// Build the actual preceding schema through the production migrator, then
// exercise the user migration with existing credentials and personal data.
func TestMigration97LegacyAdmin(t *testing.T) {
	for _, credentials := range []bool{false, true} {
		name := "passwordless"
		if credentials {
			name = "credentials"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			setupCtx := models.WithSetupContext(ctx)
			dir := t.TempDir()
			cfg := config.InitializeEmpty()
			cfg.SetConfigFile(filepath.Join(dir, "config.yml"))
			cfg.SetString(config.JWTSignKey, "migration-test-signing-key")
			username := "admin"
			var apiKey string
			if credentials {
				username = "legacy-admin"
				hash, err := bcrypt.GenerateFromPassword([]byte("legacy-password"), bcrypt.MinCost)
				require.NoError(t, err)
				cfg.SetString(config.LegacyUsername, username)
				cfg.SetString(config.LegacyPassword, string(hash))
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, user.APIKeyClaims{UserID: username, RegisteredClaims: jwt.RegisteredClaims{Subject: user.APIKeySubject}})
				apiKey, err = token.SignedString(cfg.GetJWTSignKey())
				require.NoError(t, err)
				cfg.SetString(config.LegacyAPIKey, apiKey)
			}
			require.NoError(t, cfg.Write())
			dbPath := filepath.Join(dir, "stash.sqlite")
			raw, err := sqlx.Open("sqlite3", dbPath)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, raw.Close()) })
			initial, err := os.ReadFile("1_initial.up.sql")
			require.NoError(t, err)
			_, err = raw.Exec(string(initial) + "\nCREATE TABLE schema_migrations(version uint64, dirty bool); INSERT INTO schema_migrations VALUES(1,false);")
			require.NoError(t, err)
			db := sqlite.NewDatabase()
			require.NoError(t, db.Open(setupCtx, dbPath))
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			m, err := sqlite.NewMigrator(db)
			require.NoError(t, err)
			t.Cleanup(m.Close)
			for version := uint(2); version <= 96; version++ {
				require.NoErrorf(t, m.RunMigration(setupCtx, version), "schema %d", version)
			}
			_, err = raw.Exec(`
INSERT INTO scenes(id,title,created_at,updated_at,rating,resume_time,play_duration) VALUES(1,'legacy scene',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,85,12.5,30);
INSERT INTO images(id,title,created_at,updated_at,rating,o_counter) VALUES(1,'legacy image',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,75,2);
INSERT INTO galleries(id,title,created_at,updated_at,rating,o_counter) VALUES(1,'legacy gallery',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,65,3);
INSERT INTO groups(id,name,created_at,updated_at,rating) VALUES(1,'legacy group',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,55);
INSERT INTO performers(id,name,created_at,updated_at,rating,favorite) VALUES(1,'legacy performer',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,45,1);
INSERT INTO studios(id,name,created_at,updated_at,rating,favorite) VALUES(1,'legacy studio',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,35,1);
INSERT INTO tags(id,name,created_at,updated_at,favorite) VALUES(1,'legacy tag',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,1);
INSERT INTO scenes_o_dates(scene_id,o_date) VALUES(1,CURRENT_TIMESTAMP);
INSERT INTO scenes_view_dates(scene_id,view_date) VALUES(1,CURRENT_TIMESTAMP);
INSERT INTO saved_filters(id,name,mode) VALUES(1,'legacy filter','SCENES');
INSERT INTO voice_actors(id,name,favorite,created_at,updated_at) VALUES(1,'voice artist',1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`)
			require.NoError(t, err)
			require.NoError(t, m.RunMigration(ctx, 97))
			require.Equal(t, uint(97), m.CurrentSchemaVersion())
			var result string
			require.NoError(t, raw.Get(&result, "PRAGMA integrity_check"))
			require.Equal(t, "ok", result)
			rows, err := raw.Query("PRAGMA foreign_key_check")
			require.NoError(t, err)
			require.False(t, rows.Next(), "all migrated personal data belongs to the initial admin")
			require.NoError(t, rows.Close())
			for _, table := range []string{"scenes_user_data", "images_user_data", "galleries_user_data", "groups_user_data", "performers_user_data", "studios_user_data", "tags_user_data", "voice_actors_user_data", "scenes_o_dates", "scenes_view_dates", "saved_filters"} {
				var owner int
				require.NoError(t, raw.Get(&owner, "SELECT user_id FROM "+table))
				require.Equal(t, 1, owner, table)
			}
			var counter, rating, count int
			require.NoError(t, raw.Get(&counter, "SELECT o_counter FROM galleries_user_data"))
			require.Equal(t, 3, counter)
			require.NoError(t, raw.Get(&rating, "SELECT rating FROM scenes_user_data"))
			require.Equal(t, 85, rating)
			require.NoError(t, raw.Get(&count, "SELECT count(*) FROM voice_actors"))
			require.Equal(t, 1, count)
			service := &user.Service{Store: db.User(), Config: cfg}
			require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
				require.NoError(t, service.Init(ctx))
				admin, err := service.GetUser(ctx, username)
				require.NoError(t, err)
				require.Equal(t, 1, admin.ID)
				require.True(t, admin.Roles.HasRole(models.RoleEnumAdmin))
				if credentials {
					require.NoError(t, service.ValidateCredentials(ctx, username, "legacy-password"))
					require.ErrorIs(t, service.ValidateCredentials(ctx, username, "wrong"), user.ErrAccessDenied)
					keyUser, err := service.AuthenticateByAPIKey(ctx, apiKey)
					require.NoError(t, err)
					require.Equal(t, admin.ID, keyUser.ID)
				} else {
					require.True(t, service.IsSingleUserMode())
				}
				return nil
			}))
			if credentials {
				saved, err := os.ReadFile(cfg.GetConfigFile())
				require.NoError(t, err)
				require.Contains(t, string(saved), "public_access: true")
				require.Empty(t, cfg.GetLegacyUsername())
				backups, err := filepath.Glob(cfg.GetConfigFile() + ".96.*")
				require.NoError(t, err)
				require.Len(t, backups, 1)
			}
		})
	}
}

func TestResetKeepsAccounts(t *testing.T) {
	ctx := context.Background()
	config.InitializeEmpty()
	db := sqlite.NewDatabase()
	require.NoError(t, db.Open(ctx, filepath.Join(t.TempDir(), "stash.sqlite")))
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	admins := models.Roles{models.RoleEnumAdmin}
	now := time.Now().UTC().Truncate(time.Second)
	first := models.User{Username: "first-admin", Roles: admins, CreatedAt: now, UpdatedAt: now}
	second := models.User{Username: "listener", Roles: models.Roles{models.RoleEnumRead}, Notes: "kept", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		if err := db.User().Create(ctx, &first, "hash-one"); err != nil {
			return err
		}
		if err := db.User().SetUserAPIKey(ctx, first.ID, "key-one"); err != nil {
			return err
		}
		return db.User().Create(ctx, &second, "hash-two")
	}))

	before := map[string]*models.User{}
	require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
		all, err := db.User().All(ctx)
		require.NoError(t, err)
		for _, u := range all {
			before[u.Username] = u
		}
		return nil
	}))
	require.NoError(t, db.Reset())

	require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
		all, err := db.User().All(ctx)
		require.NoError(t, err)
		require.Len(t, all, len(before))
		for _, u := range all {
			was := before[u.Username]
			require.NotNil(t, was, u.Username)
			require.Equal(t, was.ID, u.ID)
			require.Equal(t, was.ApiKeyHash, u.ApiKeyHash)
			require.Equal(t, was.Notes, u.Notes)
			require.Equal(t, was.Locked, u.Locked)
			require.ElementsMatch(t, was.Roles, u.Roles)
			require.True(t, was.UpdatedAt.Equal(u.UpdatedAt), "ending old sessions depends on this time")
		}
		hash, err := db.User().GetPasswordHash(ctx, first.ID)
		require.NoError(t, err)
		require.Equal(t, "hash-one", hash)
		require.Equal(t, "key-one", before["first-admin"].ApiKeyHash)
		return nil
	}))

	// a new account doesn't collide with the restored ids
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		return db.User().Create(ctx, &models.User{Username: "after-reset", Roles: admins, CreatedAt: now, UpdatedAt: now}, "hash-three")
	}))
}
