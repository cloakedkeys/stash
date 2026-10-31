//go:build integration

package postgres

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/signedurl"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stashapp/stash/pkg/user"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func migrationFixture(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		t.Skip("PGSQL_TEST is required")
	}
	admin, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	schema := fmt.Sprintf("migration_users_%d", time.Now().UnixNano())
	_, err = admin.Exec("CREATE SCHEMA " + schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		require.NoError(t, err)
		require.NoError(t, admin.Close())
	})
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func TestNativeUserMigrationAndSetup(t *testing.T) {
	for _, setup := range []bool{false, true} {
		name := "upgrade13"
		if setup {
			name = "fresh_setup"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			dsn := migrationFixture(t)
			cfg := config.InitializeEmpty()
			cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
			cfg.SetString(config.JWTSignKey, "postgres-migration-signing-secret")
			cfg.SetSessionStoreKey("postgres-migration-session-secret")
			cfg.SetInt(config.MaxSessionAge, 3600)
			username, password := "initial-admin", "new-password"
			var legacyKey string
			db := NewDatabase()
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			if setup {
				require.NoError(t, db.Open(models.WithSetupContext(ctx), dsn))
			} else {
				username, password = "legacy-admin", "legacy-password"
				hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
				require.NoError(t, err)
				cfg.SetString(config.LegacyUsername, username)
				cfg.SetString(config.LegacyPassword, string(hash))
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, user.APIKeyClaims{UserID: username, RegisteredClaims: jwt.RegisteredClaims{Subject: user.APIKeySubject}})
				legacyKey, err = token.SignedString(cfg.GetJWTSignKey())
				require.NoError(t, err)
				cfg.SetString(config.LegacyAPIKey, legacyKey)
				require.NoError(t, cfg.Write())
				db.dbPath = dsn
				m, err := NewMigrator(db)
				require.NoError(t, err)
				for version := uint(1); version <= 13; version++ {
					require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
				}
				_, err = m.conn.Exec(`INSERT INTO scenes(id,title,created_at,updated_at,rating,resume_time,play_duration) VALUES(1,'legacy',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,85,12.5,30);
INSERT INTO scenes_o_dates(scene_id,o_date) VALUES(1,CURRENT_TIMESTAMP);
INSERT INTO scenes_view_dates(scene_id,view_date) VALUES(1,CURRENT_TIMESTAMP);
INSERT INTO galleries(id,title,created_at,updated_at,rating) VALUES(1,'gallery',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,65);`)
				require.NoError(t, err)
				require.NoError(t, m.RunMigration(ctx, 14))
				_, err = m.conn.Exec(`UPDATE galleries SET o_counter=3 WHERE id=1; INSERT INTO voice_actors(id,name,favorite,created_at,updated_at) VALUES(1,'artist',true,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`)
				require.NoError(t, err)
				require.NoError(t, m.RunMigration(ctx, 15))
				require.NoError(t, m.RunMigration(ctx, 16))
				m.Close()
				require.NoError(t, db.ReInitialise())
				var owner int
				require.NoError(t, db.readDB.Get(&owner, "SELECT user_id FROM scenes_user_data WHERE scene_id=1"))
				require.Equal(t, 1, owner)
				require.Empty(t, cfg.GetLegacyUsername())
				require.True(t, cfg.GetPublicAccess())
				backups, err := filepath.Glob(cfg.GetConfigFile() + ".pg14.*")
				require.NoError(t, err)
				require.Len(t, backups, 1)
			}
			service := &user.Service{Store: db.User(), Config: cfg}
			require.NoError(t, txn.WithReadTxn(ctx, db, service.Init))
			if setup {
				require.NoError(t, txn.WithTxn(session.SetCurrentUser(ctx, models.User{Username: "setup"}), db, func(ctx context.Context) error {
					return service.CreateUser(ctx, models.User{Username: username, Roles: models.Roles{models.RoleEnumAdmin}}, password)
				}))
			}
			require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
				admin, err := service.GetUser(ctx, username)
				require.NoError(t, err)
				require.Equal(t, 1, admin.ID)
				require.False(t, admin.Locked)
				require.True(t, admin.Roles.HasRole(models.RoleEnumAdmin))
				require.NoError(t, service.ValidateCredentials(ctx, username, password))
				if legacyKey != "" {
					authenticated, err := service.AuthenticateByAPIKey(ctx, legacyKey)
					require.NoError(t, err)
					require.Equal(t, admin.ID, authenticated.ID)
				}
				if !setup {
					adminCtx := session.SetCurrentUser(ctx, *admin)
					g, err := db.Gallery().Find(adminCtx, 1)
					require.NoError(t, err)
					require.Equal(t, 3, g.OCounter)
					actor, err := db.VoiceActor().Find(adminCtx, 1)
					require.NoError(t, err)
					require.True(t, actor.Favorite)
					guestCtx := models.WithUserID(ctx, 0)
					g, err = db.Gallery().Find(guestCtx, 1)
					require.NoError(t, err)
					require.Zero(t, g.OCounter)
					actor, err = db.VoiceActor().Find(guestCtx, 1)
					require.NoError(t, err)
					require.False(t, actor.Favorite)
				}
				return nil
			}))
			repo := db.Repository()
			sessions := session.NewStore(cfg, service, &repo.TxnManager)
			request := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"username": {username}, "password": {password}}.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			require.NoError(t, sessions.Login(response, request))
			require.NotEmpty(t, response.Result().Cookies())
			request = httptest.NewRequest("GET", "/graphql", nil)
			request.AddCookie(response.Result().Cookies()[0])
			cookie, err := sessions.GetSession(httptest.NewRecorder(), request)
			require.NoError(t, err)
			require.NotNil(t, cookie)
			require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
				_, err := service.AuthenticateSession(ctx, cookie.UserID, cookie.LoginTime)
				return err
			}))
			key := cfg.GetJWTSignKey()
			cid := signedurl.GenerateCredentialID(key, username)
			params := signedurl.SignPrefix(signedurl.DerivePrefix("/scene/1/stream"), key, cid, time.Now().Add(time.Minute))
			_, err = signedurl.VerifyURL("/scene/1/stream", params, key)
			require.NoError(t, err)
		})
	}
}

func TestNativeUserMigrationFailsClosed(t *testing.T) {
	ctx := context.Background()
	cfg := config.InitializeEmpty()
	// The backup can be written next to this directory, but replacing the
	// config directory itself must fail after the migration clears the fields.
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
	require.NoError(t, os.Mkdir(cfg.GetConfigFile(), 0700))
	cfg.SetString(config.LegacyUsername, "legacy-admin")
	hash, err := bcrypt.GenerateFromPassword([]byte("legacy-password"), bcrypt.MinCost)
	require.NoError(t, err)
	cfg.SetString(config.LegacyPassword, string(hash))
	db := NewDatabase()
	db.dbPath = migrationFixture(t)
	m, err := NewMigrator(db)
	require.NoError(t, err)
	t.Cleanup(m.Close)
	for version := uint(1); version <= 14; version++ {
		require.NoError(t, m.RunMigration(ctx, version))
	}
	require.Error(t, m.RunMigration(ctx, 15))
	var locked bool
	var passwordCount int
	require.NoError(t, m.conn.Get(&locked, "SELECT locked FROM users WHERE id=1"))
	require.True(t, locked, "failed migration must not expose a passwordless administrator")
	require.NoError(t, m.conn.Get(&passwordCount, "SELECT count(*) FROM users WHERE password_hash IS NOT NULL"))
	require.Zero(t, passwordCount, "credential update must roll back with config failure")
	require.Equal(t, "legacy-admin", cfg.GetLegacyUsername())
	require.Equal(t, string(hash), cfg.GetLegacyPasswordHash())
	require.False(t, cfg.GetPublicAccess())
}
