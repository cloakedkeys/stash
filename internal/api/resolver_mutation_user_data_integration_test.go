//go:build integration

package api

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/postgres"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/require"

	// registers the SQLite post-migrations
	_ "github.com/stashapp/stash/pkg/sqlite/migrations"
)

// openUserDataTestDatabase opens a new SQLite database, or a throwaway PostgreSQL
// database when PGSQL_TEST is set.
func openUserDataTestDatabase(ctx context.Context, t *testing.T) database.Database {
	t.Helper()
	config.InitializeEmpty()

	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		db := sqlite.NewDatabase()
		require.NoError(t, db.Open(ctx, filepath.Join(t.TempDir(), "stash.sqlite")))
		t.Cleanup(func() { require.NoError(t, db.Close()) })
		return db
	}

	admin, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	require.NoError(t, err)
	name := fmt.Sprintf("user_data_missing_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.Exec("DROP DATABASE " + name)
		require.NoError(t, err)
		require.NoError(t, admin.Close())
	})
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/" + name

	db := postgres.NewDatabase()
	require.NoError(t, db.Open(ctx, u.String()))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

// Runs on SQLite, or PostgreSQL when PGSQL_TEST is set.
func TestUserDataUpdateMissingObjectOnDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := openUserDataTestDatabase(ctx, t)

	user := models.User{Username: "rater", Roles: models.Roles{models.RoleEnumRead}}
	performer := models.NewPerformer()
	performer.Name = "Someone"
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		if err := db.User().Create(ctx, &user, ""); err != nil {
			return err
		}
		return db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &performer})
	}))

	r := &Resolver{repository: db.Repository(), hookExecutor: &mockHookExecutor{}}
	gql := newTestGraphQLServer(r)
	gql.AroundFields(r.LibraryLimitMiddleware)

	// the database's own error used to come back for all of these
	for mutation, want := range userDataMissingMutations {
		body := storyRequest(t, gql, user, "mutation { "+mutation+" }")
		require.Contains(t, body, `"message":"`+want+`"`, mutation)
		require.NotContains(t, body, "FOREIGN KEY", mutation)
		require.NotContains(t, body, "foreign key", mutation)
	}

	// an object that exists is rated as before
	body := storyRequest(t, gql, user, fmt.Sprintf(`mutation { performerUserDataUpdate(input:{id:"%d",rating100:80,favorite:true}) { rating100 favorite } }`, performer.ID))
	require.JSONEq(t, `{"data":{"performerUserDataUpdate":{"rating100":80,"favorite":true}}}`, body)
}
