package plugin

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/plugin/hook"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type jsUserTestConfig struct {
	pluginInputTestConfig
	pluginsPath string
}

func (c *jsUserTestConfig) GetPluginsPath() string { return c.pluginsPath }

type jsUserTestSessionConfig struct{}

func (jsUserTestSessionConfig) GetSessionStoreKey() []byte {
	return []byte("0123456789abcdef0123456789abcdef")
}
func (jsUserTestSessionConfig) GetMaxSessionAge() int { return 3600 }

// jsUserRecorder is the in-process GraphQL handler. It notes the user each
// call carries, as the real handler sees it.
type jsUserRecorder struct {
	mu    sync.Mutex
	calls int
	users []*models.User
}

func (h *jsUserRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.calls++
	h.users = append(h.users, session.GetCurrentUser(r.Context()))
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"data":{"me":null}}`))
}

func (h *jsUserRecorder) only(t *testing.T) *models.User {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	require.Equal(t, 1, h.calls, "the plugin should call GraphQL once")
	return h.users[0]
}

func newJSUserTestCache(t *testing.T) (*Cache, *jsUserRecorder) {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "jsuser")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "jsuser.yml"), []byte(`name: JS user
exec:
  - jsuser.js
interface: js
tasks:
  - name: Who
    description: asks who it is
hooks:
  - name: Who on update
    triggeredBy:
      - Scene.Update.Post
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "jsuser.js"), []byte(`
var res = gql.Do("query { me { username } }");
({ Output: res });
`), 0644))

	cache := NewCache(&jsUserTestConfig{pluginsPath: dir})
	cache.RegisterSessionStore(session.NewStore(jsUserTestSessionConfig{}, nil, nil))
	recorder := &jsUserRecorder{}
	cache.RegisterGQLHandler(recorder)
	cache.ReloadPlugins()
	require.NotNil(t, cache.getPlugin("jsuser"))
	return cache, recorder
}

// #2116: a JavaScript plugin's GraphQL calls carry the user whose request
// started it, however it was started.
func TestJSPluginActsAsStartingUser(t *testing.T) {
	limit := &models.LibraryLimit{TagIDs: []int{7}}
	starter := models.User{ID: 4, Username: "viewer", Roles: models.Roles{models.RoleEnumRead}, LibraryLimit: limit}

	starts := map[string]func(ctx context.Context, c *Cache) error{
		"runPluginOperation": func(ctx context.Context, c *Cache) error {
			_, err := c.RunPlugin(ctx, "jsuser", nil)
			return err
		},
		"runPluginTask": func(ctx context.Context, c *Cache) error {
			name := "Who"
			task, err := c.CreateTask(ctx, "jsuser", &name, nil, make(chan float64, 10))
			if err != nil {
				return err
			}
			if err := task.Start(); err != nil {
				return err
			}
			task.Wait()
			return nil
		},
		"hook": func(ctx context.Context, c *Cache) error {
			c.ExecutePostHooks(ctx, 1, hook.SceneUpdatePost, nil, nil)
			return nil
		},
	}

	for name, start := range starts {
		t.Run(name, func(t *testing.T) {
			cache, recorder := newJSUserTestCache(t)
			ctx := session.SetCurrentUser(context.Background(), starter)
			require.NoError(t, start(ctx, cache))

			got := recorder.only(t)
			require.NotNil(t, got, "the plugin ran with no user")
			assert.Equal(t, starter.ID, got.ID)
			assert.Equal(t, starter.Username, got.Username)
			assert.Equal(t, starter.Roles, got.Roles)
			assert.Equal(t, limit, got.LibraryLimit)
		})

		// nobody started it: there is still no user here; in single-user mode
		// the server's handler fills in the single user
		t.Run(name+" without a user", func(t *testing.T) {
			cache, recorder := newJSUserTestCache(t)
			require.NoError(t, start(context.Background(), cache))
			assert.Nil(t, recorder.only(t))
		})
	}
}

// The plugin keeps the user, not the request: a request that has finished or
// been cancelled doesn't cancel the plugin's own calls.
func TestJSPluginOutlivesStartingRequest(t *testing.T) {
	cache, recorder := newJSUserTestCache(t)
	starter := models.User{ID: 2, Username: "admin", Roles: models.Roles{models.RoleEnumAdmin}}
	ctx, cancel := context.WithCancel(session.SetCurrentUser(context.Background(), starter))

	name := "Who"
	task, err := cache.CreateTask(ctx, "jsuser", &name, nil, make(chan float64, 10))
	require.NoError(t, err)
	cancel()
	require.NoError(t, task.Start())
	task.Wait()

	require.Nil(t, task.GetResult().Error)
	got := recorder.only(t)
	require.NotNil(t, got)
	assert.Equal(t, "admin", got.Username)
}
