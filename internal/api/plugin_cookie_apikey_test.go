package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/securecookie"
	gorillasessions "github.com/gorilla/sessions"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stretchr/testify/require"
)

// TestPluginCookieEndsWithAPIKey checks that deleting or replacing an API key
// ends the plugins started with it (#2122), while the user's own browser
// sessions and other users' keys keep working.
func TestPluginCookieEndsWithAPIKey(t *testing.T) {
	const cookieSecret = "plugin-cookie-apikey-test-secret"

	cfg := config.InitializeEmpty()
	cfg.SetPublicAccess(true)
	cfg.SetString(config.SessionStoreKey, cookieSecret)
	cfg.SetInt(config.MaxSessionAge, 3600)
	db := mocks.NewDatabase()
	tx := db.Repository().TxnManager

	now := time.Now()
	loginTime := now.Add(-2 * time.Hour)
	notUpdatedSinceLogin := now.Add(-3 * time.Hour)

	users := &pluginCookieUsers{
		admin: &models.User{ID: 1, Username: "admin", Roles: models.Roles{models.RoleEnumAdmin}},
		other: &models.User{ID: 2, Username: "other", Roles: models.Roles{models.RoleEnumModify}},
	}
	reset := func() {
		users.admin.ApiKeyHash = "admin-key-hash"
		users.admin.UpdatedAt = notUpdatedSinceLogin
		users.other.ApiKeyHash = "other-key-hash"
		users.other.UpdatedAt = notUpdatedSinceLogin
	}
	deleteAdminKey := func() { users.admin.ApiKeyHash = "" }

	sessions := session.NewStore(cfg, users, &tx)

	// browserCookie is the login cookie of the admin's own browser session,
	// in the format Login writes.
	browserCookie := func() *http.Cookie {
		t.Helper()
		codecs := gorillasessions.NewCookieStore(cfg.GetSessionStoreKey()).Codecs
		encoded, err := securecookie.EncodeMulti("session", map[interface{}]interface{}{
			"userID":         "admin",
			"loginTime":      loginTime.Unix(),
			"loginTimeNanos": loginTime.UnixNano(),
		}, codecs...)
		require.NoError(t, err)
		return &http.Cookie{Name: "session", Value: encoded}
	}

	// The handler stands in for a hook or plugin task: it reports who the
	// request runs as, makes the plugin cookie and keeps the request context,
	// which a queued task uses when it starts later.
	var pluginCookie *http.Cookie
	var requestCtx context.Context
	h := authenticateHandlerWithServices(tx, cfg, func() UserAuthenticator { return users }, sessions)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			name := "none"
			if u := session.GetCurrentUser(r.Context()); u != nil {
				name = u.Username
			}
			requestCtx = r.Context()
			pluginCookie = sessions.MakePluginCookie(r.Context())
			fmt.Fprint(w, name)
		}))

	request := func(key string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		pluginCookie = nil
		requestCtx = nil
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
		h.ServeHTTP(w, req)
		return w
	}

	// startPlugin signs in the way the caller says and returns the cookie the
	// plugin is given.
	startPlugin := func(key string, cookie *http.Cookie) *http.Cookie {
		t.Helper()
		w := request(key, cookie)
		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, pluginCookie)
		return pluginCookie
	}

	requireCallsBackAs := func(c *http.Cookie, want string, msg string) {
		t.Helper()
		w := request("", c)
		require.Equal(t, http.StatusOK, w.Code, msg)
		require.Equal(t, want, w.Body.String(), msg)
	}

	t.Run("deleting the key ends the plugin cookie", func(t *testing.T) {
		reset()
		c := startPlugin("admin-key", nil)
		requireCallsBackAs(c, "admin", "the plugin works while the key exists")

		deleteAdminKey()

		require.Equal(t, http.StatusUnauthorized, request("admin-key", nil).Code, "the key itself is refused")
		require.Equal(t, http.StatusUnauthorized, request("", c).Code,
			"the plugin cookie must be refused once its key is deleted")
	})

	t.Run("replacing the key ends the plugin cookie", func(t *testing.T) {
		reset()
		c := startPlugin("admin-key", nil)

		users.admin.ApiKeyHash = "replacement-key-hash"

		require.Equal(t, http.StatusUnauthorized, request("", c).Code,
			"the plugin cookie must be refused once its key is replaced")
	})

	t.Run("queued task started after the key is deleted", func(t *testing.T) {
		reset()
		w := request("admin-key", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, requestCtx)
		queuedCtx := requestCtx

		deleteAdminKey()

		// the task starts and makes its plugin cookie now
		c := sessions.MakePluginCookie(queuedCtx)
		require.NotNil(t, c)
		require.Equal(t, http.StatusUnauthorized, request("", c).Code,
			"a task queued with a deleted key must not call back")
	})

	t.Run("hook started by the plugin's own call back", func(t *testing.T) {
		reset()
		c := startPlugin("admin-key", nil)
		// the plugin calls back, and that request starts a hook with a new cookie
		hookCookie := startPlugin("", c)
		requireCallsBackAs(hookCookie, "admin", "the hook works while the key exists")

		deleteAdminKey()

		require.Equal(t, http.StatusUnauthorized, request("", hookCookie).Code,
			"a cookie made from a key's plugin cookie must end with the key")
	})

	t.Run("the user's browser session is unaffected", func(t *testing.T) {
		reset()
		browserPlugin := startPlugin("", browserCookie())

		deleteAdminKey()

		requireCallsBackAs(browserCookie(), "admin", "deleting a key must not log the user out")
		requireCallsBackAs(browserPlugin, "admin", "plugins started from the browser keep working")
		requireCallsBackAs(startPlugin("", browserCookie()), "admin", "and new ones start as before")
	})

	t.Run("other keys are unaffected", func(t *testing.T) {
		reset()
		otherPlugin := startPlugin("other-key", nil)

		deleteAdminKey()

		require.Equal(t, http.StatusOK, request("other-key", nil).Code)
		requireCallsBackAs(otherPlugin, "other", "another user's key plugin keeps working")
	})

	t.Run("a new key works for new plugins", func(t *testing.T) {
		reset()
		old := startPlugin("admin-key", nil)

		// delete the key, then give the user a key again
		deleteAdminKey()
		require.Equal(t, http.StatusUnauthorized, request("", old).Code)
		users.admin.ApiKeyHash = "admin-key-hash"

		requireCallsBackAs(startPlugin("admin-key", nil), "admin", "a plugin started with the current key works")
	})
}
