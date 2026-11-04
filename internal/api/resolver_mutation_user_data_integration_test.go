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
	"github.com/stashapp/stash/pkg/group"
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
	editor := models.User{Username: "editor", Roles: models.Roles{models.RoleEnumModify}}
	performer := models.NewPerformer()
	performer.Name = "Someone"
	scene := models.Scene{Title: "A scene"}
	img := models.Image{Title: "An image"}
	grp := models.Group{Name: "A group"}
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		if err := db.User().Create(ctx, &user, ""); err != nil {
			return err
		}
		if err := db.User().Create(ctx, &editor, ""); err != nil {
			return err
		}
		if err := db.Scene().Create(ctx, &scene, nil); err != nil {
			return err
		}
		if err := db.Image().Create(ctx, &models.CreateImageInput{Image: &img}); err != nil {
			return err
		}
		if err := db.Group().Create(ctx, &grp); err != nil {
			return err
		}
		return db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &performer})
	}))

	r := &Resolver{repository: db.Repository(), hookExecutor: &mockHookExecutor{}, groupService: &group.Service{Repository: db.Group()}}
	gql := newTestGraphQLServer(r)
	gql.AroundFields(r.LibraryLimitMiddleware)

	// the database's own error, or one naming its table, used to come back for all of these
	check := func(u models.User, mutations map[string]string) {
		for mutation, want := range mutations {
			t.Run(mutation, func(t *testing.T) {
				body := storyRequest(t, gql, u, "mutation { "+mutation+" }")
				require.Contains(t, body, `"message":"`+want+`"`)
				require.NotContains(t, body, "FOREIGN KEY")
				require.NotContains(t, body, "foreign key")
				require.NotContains(t, body, "does not exist")
			})
		}
	}
	check(user, userDataMissingMutations)
	check(editor, missingObjectUpdateMutations)

	// removing or resetting on a missing scene or track gives 0, as upstream does
	for mutation, want := range missingObjectNoOpMutations {
		t.Run(mutation, func(t *testing.T) {
			body := storyRequest(t, gql, user, "mutation { "+mutation+" }")
			require.JSONEq(t, want, body)
		})
	}

	// an object that exists is rated as before
	body := storyRequest(t, gql, user, fmt.Sprintf(`mutation { performerUserDataUpdate(input:{id:"%d",rating100:80,favorite:true}) { rating100 favorite } }`, performer.ID))
	require.JSONEq(t, `{"data":{"performerUserDataUpdate":{"rating100":80,"favorite":true}}}`, body)

	// and O and plays are counted as before
	body = storyRequest(t, gql, user, fmt.Sprintf(`mutation { sceneAddO(id:"%d") { count } }`, scene.ID))
	require.JSONEq(t, `{"data":{"sceneAddO":{"count":1}}}`, body)
	body = storyRequest(t, gql, user, fmt.Sprintf(`mutation { sceneAddPlay(id:"%d") { count } }`, scene.ID))
	require.JSONEq(t, `{"data":{"sceneAddPlay":{"count":1}}}`, body)
	body = storyRequest(t, gql, user, fmt.Sprintf(`mutation { sceneSaveActivity(id:"%d",resume_time:5,playDuration:5) }`, scene.ID))
	require.JSONEq(t, `{"data":{"sceneSaveActivity":true}}`, body)
	body = storyRequest(t, gql, user, fmt.Sprintf(`mutation { imageIncrementO(id:"%d") }`, img.ID))
	require.JSONEq(t, `{"data":{"imageIncrementO":1}}`, body)

	// a group's aliases are still tidied, leaving out its name and repeats
	body = storyRequest(t, gql, editor, fmt.Sprintf(`mutation { groupUpdate(input:{id:"%d",alias_list:["Other","a group","other"]}) { alias_list } }`, grp.ID))
	require.JSONEq(t, `{"data":{"groupUpdate":{"alias_list":["Other"]}}}`, body)
}
