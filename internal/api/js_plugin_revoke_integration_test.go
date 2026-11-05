//go:build integration

package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jsRevokeAuthConfig is the server's login settings for authenticateHandler:
// a local server with accounts.
type jsRevokeAuthConfig struct{}

func (jsRevokeAuthConfig) IsNewSystem() bool { return false }
func (jsRevokeAuthConfig) GetNewSystemCredentials() (string, string, time.Time) {
	return "", "", time.Time{}
}
func (jsRevokeAuthConfig) GetPublicAccess() bool                       { return true }
func (jsRevokeAuthConfig) GetPublicWhitelist() ([]net.IPNet, []net.IP) { return nil, nil }

// #2122 and #2130: a JavaScript plugin is checked on each of its calls the way
// a returning plugin cookie is. Deleting the key that started it, locking the
// account, changing its roles or resetting its password ends its access, and
// the cookies for hooks it starts end with the key too. Runs on SQLite, or
// PostgreSQL when PGSQL_TEST is set.
func TestJSPluginLosesAccessMidRunOnDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := openUserDataTestDatabase(ctx, t)
	config.GetInstance().SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))

	boss := models.User{Username: "boss", Roles: models.Roles{models.RoleEnumAdmin}}
	other := models.User{Username: "other", Roles: models.Roles{models.RoleEnumAdmin}}
	viewer := models.User{Username: "viewer", Roles: models.Roles{models.RoleEnumRead}}
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		for _, u := range []*models.User{&boss, &other, &viewer} {
			if err := db.User().Create(ctx, u, "x"); err != nil {
				return err
			}
		}
		// a scene the viewer sees until its library limit is changed
		return db.Scene().Create(ctx, &models.Scene{Title: "A scene"}, nil)
	}))

	h := newJSPluginUserHarness(ctx, t, db, false)

	inTxn := func(fn func(ctx context.Context) error) {
		t.Helper()
		require.NoError(t, txn.WithTxn(ctx, db, fn))
	}
	newKey := func(username string) string {
		t.Helper()
		var key string
		inTxn(func(ctx context.Context) error {
			var err error
			key, err = h.users.GenerateAPIKey(ctx, username)
			return err
		})
		return key
	}
	clearKey := func(username string) {
		t.Helper()
		inTxn(func(ctx context.Context) error { return h.users.ClearAPIKey(ctx, username) })
	}

	// the real login check, for requests with a key or a cookie
	var gotCtx context.Context
	auth := authenticateHandlerWithServices(db.Repository().TxnManager, jsRevokeAuthConfig{}, func() UserAuthenticator { return h.users }, h.sessions)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotCtx = r.Context()
			name := "none"
			if u := session.GetCurrentUser(r.Context()); u != nil {
				name = u.Username
			}
			fmt.Fprint(w, name)
		}))
	request := func(key string, cookie *http.Cookie) (int, string) {
		t.Helper()
		gotCtx = nil
		req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set(session.ApiKeyHeader, key)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		req = req.WithContext(context.WithValue(req.Context(), IPCtxKey, net.ParseIP("127.0.0.1")))
		w := httptest.NewRecorder()
		auth.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	// startedWith returns the context of a request signed in with key, which
	// is what starts a plugin
	startedWith := func(key string) context.Context {
		t.Helper()
		code, body := request(key, nil)
		require.Equal(t, http.StatusOK, code, body)
		require.NotNil(t, gotCtx)
		return gotCtx
	}

	// nextSecond waits until the clock is past the next whole second. The user
	// store keeps update times to the second on SQLite, so a change in the same
	// second the plugin started can't be told apart from one before it, for
	// plugin cookies and browser sessions alike.
	nextSecond := func() {
		now := time.Now()
		time.Sleep(now.Truncate(time.Second).Add(time.Second + 10*time.Millisecond).Sub(now))
	}

	refused := func(t *testing.T, r jsPluginResult) {
		t.Helper()
		assert.Contains(t, r.Error, "401", "the call must be refused")
		assert.Empty(t, string(r.Data))
	}
	me := func(t *testing.T, r jsPluginResult, name string) {
		t.Helper()
		assert.Empty(t, r.Error)
		assert.Contains(t, string(r.Data), fmt.Sprintf(`"name":%q`, name))
	}

	t.Run("key deleted mid-run", func(t *testing.T) {
		start := startedWith(newKey("boss"))
		h.onCall = func(n int) {
			if n == 1 {
				clearKey("boss")
			}
		}
		defer func() { h.onCall = nil }()

		got := h.runFrom(start, jsMeQuery, jsMeQuery, `mutation { tagCreate(input: { name: "after key deleted" }) { id } }`)
		me(t, got[0], "boss")
		refused(t, got[1])
		refused(t, got[2])
	})

	t.Run("hook started by the plugin ends with the key", func(t *testing.T) {
		start := startedWith(newKey("boss"))
		got := h.runFrom(start, `mutation { tagCreate(input: { name: "from js plugin" }) { id } }`)
		require.Empty(t, got[0].Error)
		require.Len(t, h.hookCookies, 1)
		hookCookie := h.hookCookies[0]
		require.NotNil(t, hookCookie)

		code, body := request("", hookCookie)
		require.Equal(t, http.StatusOK, code, "the hook works while the key exists")
		require.Equal(t, "boss", body)

		clearKey("boss")

		code, _ = request("", hookCookie)
		assert.Equal(t, http.StatusUnauthorized, code, "the hook's cookie must end with the key")
	})

	t.Run("other keys and users are unaffected", func(t *testing.T) {
		start := startedWith(newKey("other"))
		newKey("boss")
		h.onCall = func(n int) {
			if n == 1 {
				clearKey("boss")
			}
		}
		defer func() { h.onCall = nil }()

		got := h.runFrom(start, jsMeQuery, jsMeQuery)
		me(t, got[0], "other")
		me(t, got[1], "other")

		// a plugin the user started without a key keeps working when the
		// key is deleted
		newKey("boss")
		got = h.run(&boss, jsMeQuery, jsMeQuery)
		me(t, got[0], "boss")
		me(t, got[1], "boss")
	})

	// t.Run order matters below: each changes the viewer for good

	t.Run("library limit change applies mid-run", func(t *testing.T) {
		h.onCall = func(n int) {
			if n == 1 {
				inTxn(func(ctx context.Context) error {
					return h.users.SetLibraryLimit(ctx, "viewer", &models.LibraryLimit{TagIDs: []int{}})
				})
			}
		}
		defer func() { h.onCall = nil }()

		vu := viewer
		got := h.run(&vu, jsSceneCount, jsSceneCount)
		assert.Empty(t, got[0].Error)
		assert.JSONEq(t, `{"findScenes":{"count":1}}`, string(got[0].Data))
		assert.Empty(t, got[1].Error)
		// the viewer can see nothing once the empty limit is set, as for its
		// own requests
		assert.JSONEq(t, `{"findScenes":{"count":0}}`, string(got[1].Data))

		inTxn(func(ctx context.Context) error { return h.users.SetLibraryLimit(ctx, "viewer", nil) })
	})

	t.Run("lock mid-run", func(t *testing.T) {
		h.onCall = func(n int) {
			if n == 1 {
				inTxn(func(ctx context.Context) error { return h.users.LockUser(ctx, "viewer") })
			}
		}
		defer func() { h.onCall = nil }()

		vu := viewer
		got := h.run(&vu, jsMeQuery, jsMeQuery)
		me(t, got[0], "viewer")
		refused(t, got[1])

		inTxn(func(ctx context.Context) error { return h.users.UnlockUser(ctx, "viewer") })
	})

	t.Run("role change mid-run", func(t *testing.T) {
		h.onCall = func(n int) {
			if n == 1 {
				nextSecond()
				inTxn(func(ctx context.Context) error {
					return h.users.UpdateUser(ctx, "viewer", models.User{Username: "viewer", Roles: models.Roles{models.RoleEnumRead, models.RoleEnumModify}})
				})
			}
		}
		defer func() { h.onCall = nil }()

		// a role change ends the user's sessions, so it ends the plugin's too
		vu := viewer
		got := h.run(&vu, jsMeQuery, jsMeQuery)
		me(t, got[0], "viewer")
		refused(t, got[1])
	})

	t.Run("password reset mid-run", func(t *testing.T) {
		h.onCall = func(n int) {
			if n == 1 {
				nextSecond()
				inTxn(func(ctx context.Context) error { return h.users.ChangeUserPassword(ctx, "viewer", "new-password") })
			}
		}
		defer func() { h.onCall = nil }()

		vu := viewer
		got := h.run(&vu, jsMeQuery, jsMeQuery)
		me(t, got[0], "viewer")
		refused(t, got[1])
	})
}
