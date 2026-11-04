//go:build integration

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/api/loaders"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/plugin"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stashapp/stash/pkg/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type jsPluginUserServerConfig struct{ pluginsPath string }

func (c jsPluginUserServerConfig) GetHost() string                              { return "localhost" }
func (c jsPluginUserServerConfig) GetPort() int                                 { return 9999 }
func (c jsPluginUserServerConfig) GetConfigPathAbs() string                     { return "" }
func (c jsPluginUserServerConfig) HasTLSConfig() bool                           { return false }
func (c jsPluginUserServerConfig) GetPluginsPath() string                       { return c.pluginsPath }
func (c jsPluginUserServerConfig) GetDisabledPlugins() []string                 { return nil }
func (c jsPluginUserServerConfig) GetPluginConfiguration(string) map[string]any { return nil }
func (c jsPluginUserServerConfig) GetPythonPath() string                        { return "" }
func (jsPluginUserServerConfig) GetSessionStoreKey() []byte {
	return []byte("0123456789abcdef0123456789abcdef")
}
func (jsPluginUserServerConfig) GetMaxSessionAge() int { return 3600 }

// the user service's settings: a local server with no guest account
func (jsPluginUserServerConfig) IsNewSystem() bool         { return false }
func (jsPluginUserServerConfig) GetPublicAccess() bool     { return false }
func (jsPluginUserServerConfig) GetGuestUserEnabled() bool { return false }
func (jsPluginUserServerConfig) SetGuestUserEnabled(bool)  {}

// jsPluginUserHarness runs a JavaScript plugin against the real schema, with
// the in-process GraphQL handler put together as the server does.
type jsPluginUserHarness struct {
	t     *testing.T
	db    database.Database
	cache *plugin.Cache
}

func newJSPluginUserHarness(ctx context.Context, t *testing.T, db database.Database, singleUserMode bool) *jsPluginUserHarness {
	t.Helper()

	dir := t.TempDir()
	cfg := jsPluginUserServerConfig{pluginsPath: dir}

	users := &user.Service{Store: db.User(), Config: cfg}
	require.NoError(t, txn.WithReadTxn(ctx, db, users.Init))
	require.Equal(t, singleUserMode, users.IsSingleUserMode())

	r := &Resolver{repository: db.Repository(), hookExecutor: &mockHookExecutor{}, userService: users}
	gql := newTestGraphQLServer(r)
	gql.AroundFields(r.LibraryLimitMiddleware)
	dataloaders := loaders.Middleware{Repository: db.Repository()}
	handler := pluginUserHandler(db.Repository().TxnManager, func() UserAuthenticator { return users })(dataloaders.Middleware(gql))

	pluginDir := filepath.Join(dir, "whoami")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "whoami.yml"), []byte("name: Who am I\nexec:\n  - whoami.js\ninterface: js\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "whoami.js"), []byte(`
var out = [];
for (var i = 0; i < input.Args.queries.length; i++) {
	try {
		out.push({ data: gql.Do(input.Args.queries[i], {}) });
	} catch (e) {
		out.push({ error: String(e) });
	}
}
({ Output: out });
`), 0644))

	cache := plugin.NewCache(cfg)
	cache.RegisterSessionStore(session.NewStore(cfg, nil, nil))
	cache.RegisterGQLHandler(handler)
	cache.ReloadPlugins()

	return &jsPluginUserHarness{t: t, db: db, cache: cache}
}

type jsPluginResult struct {
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

// run runs the plugin as the given user, or as nobody, and returns what each
// query gave the plugin.
func (h *jsPluginUserHarness) run(u *models.User, queries ...string) []jsPluginResult {
	h.t.Helper()
	ctx := context.Background()
	if u != nil {
		ctx = session.SetCurrentUser(ctx, *u)
	}

	list := make([]interface{}, len(queries))
	for i, q := range queries {
		list[i] = q
	}
	out, err := h.cache.RunPlugin(ctx, "whoami", plugin.OperationInput{"queries": list})
	require.NoError(h.t, err)

	b, err := json.Marshal(out)
	require.NoError(h.t, err)
	var ret []jsPluginResult
	require.NoError(h.t, json.Unmarshal(b, &ret))
	require.Len(h.t, ret, len(queries))
	return ret
}

func (h *jsPluginUserHarness) uiConfig(u models.User) map[string]interface{} {
	h.t.Helper()
	var stored map[string]interface{}
	require.NoError(h.t, txn.WithReadTxn(context.Background(), h.db, func(ctx context.Context) error {
		var err error
		stored, err = h.db.UserUIConfig().Get(models.WithUserID(ctx, u.ID))
		return err
	}))
	return stored
}

const (
	jsMeQuery        = `query { me { name roles } }`
	jsUISettingQuery = `mutation { configureUISetting(key: "fromPlugin", value: 3) }`
	jsSceneCount     = `query { findScenes { count } }`
)

// #2116: in single-user mode a JavaScript plugin acts as the single user,
// whether a request started it or nothing did, so its per-user calls work as
// they do upstream. Runs on SQLite, or PostgreSQL when PGSQL_TEST is set.
func TestJSPluginActsAsSingleUserOnDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := openUserDataTestDatabase(ctx, t)
	config.GetInstance().SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))

	// a new database has its first account already, with no password
	var solo models.User
	sc := models.Scene{Title: "A scene"}
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		all, err := db.User().All(ctx)
		if err != nil {
			return err
		}
		require.Len(t, all, 1)
		solo = *all[0]
		return db.Scene().Create(ctx, &sc, nil)
	}))

	h := newJSPluginUserHarness(ctx, t, db, true)

	// a scheduled task or startup hook: nobody started it
	for name, starter := range map[string]*models.User{"no request": nil, "request": &solo} {
		t.Run(name, func(t *testing.T) {
			got := h.run(starter, jsMeQuery, jsUISettingQuery, fmt.Sprintf(`mutation { sceneAddO(id: "%d") { count } }`, sc.ID))
			assert.Empty(t, got[0].Error)
			assert.JSONEq(t, fmt.Sprintf(`{"me":{"name":%q,"roles":["ADMIN"]}}`, solo.Username), string(got[0].Data))
			assert.Empty(t, got[1].Error)
			assert.Empty(t, got[2].Error)
		})
	}

	assert.Equal(t, 3.0, h.uiConfig(solo)["fromPlugin"])
	got := h.run(nil, fmt.Sprintf(`query { findScene(id: "%d") { o_counter } }`, sc.ID))
	assert.JSONEq(t, `{"findScene":{"o_counter":2}}`, string(got[0].Data))
}

// #2116: with several accounts a JavaScript plugin acts as the account whose
// request started it, with that account's library limit. With no account it
// runs as before: no user. Runs on SQLite, or PostgreSQL when PGSQL_TEST is set.
func TestJSPluginActsAsStartingUserOnDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := openUserDataTestDatabase(ctx, t)
	config.GetInstance().SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))

	admin := models.User{Username: "boss", Roles: models.Roles{models.RoleEnumAdmin}}
	viewer := models.User{Username: "viewer", Roles: models.Roles{models.RoleEnumRead}}
	allowed := models.Tag{Name: "Allowed"}
	var seen, hidden models.Scene
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		// a password hash makes these real accounts, not single-user mode
		if err := db.User().Create(ctx, &admin, "x"); err != nil {
			return err
		}
		if err := db.User().Create(ctx, &viewer, "x"); err != nil {
			return err
		}
		if err := db.Tag().Create(ctx, &models.CreateTagInput{Tag: &allowed}); err != nil {
			return err
		}
		seen = models.Scene{Title: "Seen", TagIDs: models.NewRelatedIDs([]int{allowed.ID})}
		hidden = models.Scene{Title: "Hidden"}
		if err := db.Scene().Create(ctx, &seen, nil); err != nil {
			return err
		}
		if err := db.Scene().Create(ctx, &hidden, nil); err != nil {
			return err
		}
		return db.User().SetLibraryLimit(ctx, viewer.ID, &models.LibraryLimit{TagIDs: []int{allowed.ID}})
	}))
	viewer.LibraryLimit = &models.LibraryLimit{TagIDs: []int{allowed.ID}}

	h := newJSPluginUserHarness(ctx, t, db, false)

	addO := func(sc models.Scene) string { return fmt.Sprintf(`mutation { sceneAddO(id: "%d") { count } }`, sc.ID) }

	t.Run("limited account", func(t *testing.T) {
		got := h.run(&viewer, jsMeQuery, jsUISettingQuery, jsSceneCount, addO(seen), addO(hidden), `mutation { reloadPlugins }`)
		assert.Empty(t, got[0].Error)
		assert.JSONEq(t, `{"me":{"name":"viewer","roles":["READ"]}}`, string(got[0].Data))
		assert.Empty(t, got[1].Error)
		assert.JSONEq(t, `{"findScenes":{"count":1}}`, string(got[2].Data))
		assert.Empty(t, got[3].Error)
		assert.JSONEq(t, `{"sceneAddO":{"count":1}}`, string(got[3].Data))
		// outside the limit, as for the account's own requests
		assert.Contains(t, got[4].Error, `"message":"not found"`)
		// an administrator's mutation, refused to a read-only account
		assert.Contains(t, got[5].Error, `"message":"unauthorized"`)

		assert.Equal(t, 3.0, h.uiConfig(viewer)["fromPlugin"])
		assert.Nil(t, h.uiConfig(admin)["fromPlugin"])
	})

	t.Run("admin", func(t *testing.T) {
		got := h.run(&admin, jsMeQuery, jsSceneCount, addO(hidden))
		assert.Empty(t, got[0].Error)
		assert.JSONEq(t, `{"me":{"name":"boss","roles":["ADMIN"]}}`, string(got[0].Data))
		assert.JSONEq(t, `{"findScenes":{"count":2}}`, string(got[1].Data))
		assert.JSONEq(t, `{"sceneAddO":{"count":1}}`, string(got[2].Data))
	})

	// nobody started it: as before, there is no user, so per-user calls fail
	// and the role checks and library limits have no one to apply to
	t.Run("no request", func(t *testing.T) {
		got := h.run(nil, jsMeQuery, jsUISettingQuery, jsSceneCount)
		assert.Empty(t, got[0].Error)
		assert.JSONEq(t, `{"me":null}`, string(got[0].Data))
		assert.Contains(t, got[1].Error, "no current user")
		assert.JSONEq(t, `{"findScenes":{"count":2}}`, string(got[2].Data))
	})
}
