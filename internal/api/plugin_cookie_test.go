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
	"github.com/stashapp/stash/pkg/user"
	"github.com/stretchr/testify/require"
)

// pluginCookieUsers applies the same revocation rule as user.Service: a
// session is refused when the user was updated after its login time.
type pluginCookieUsers struct {
	apiTestUsers
	admin *models.User
}

func (s *pluginCookieUsers) AuthenticateByAPIKey(_ context.Context, key string) (*models.User, error) {
	if key == "admin-key" {
		u := *s.admin
		return &u, nil
	}
	return nil, user.ErrAccessDenied
}

func (s *pluginCookieUsers) AuthenticateSession(_ context.Context, username string, loginTime time.Time) (*models.User, error) {
	if username != s.admin.Username || s.admin.UpdatedAt.After(loginTime) {
		return nil, user.ErrAccessDenied
	}
	u := *s.admin
	return &u, nil
}

// TestPluginCookieAuthenticatesCallback checks that the cookie handed to a
// plugin lets the plugin call back as the same user, like upstream, while a
// revoked session stays revoked.
//
// The user's session logs in two hours before the test runs, so its login
// time can never be confused with the time the plugin cookie is made.
func TestPluginCookieAuthenticatesCallback(t *testing.T) {
	const cookieSecret = "plugin-cookie-test-cookie-secret"

	cfg := config.InitializeEmpty()
	cfg.SetPublicAccess(true)
	cfg.SetString(config.SessionStoreKey, cookieSecret)
	cfg.SetInt(config.MaxSessionAge, 3600)
	db := mocks.NewDatabase()
	tx := db.Repository().TxnManager

	now := time.Now()
	loginTime := now.Add(-2 * time.Hour).Truncate(time.Second).Add(123456789 * time.Nanosecond)
	notUpdatedSinceLogin := now.Add(-3 * time.Hour)
	updatedSinceLogin := now.Add(-time.Hour)

	users := &pluginCookieUsers{admin: &models.User{
		ID:       1,
		Username: "admin",
		Roles:    models.Roles{models.RoleEnumAdmin},
	}}
	sessions := session.NewStore(cfg, users, &tx)

	// userSessionCookie is the browser cookie of a session that logged in at
	// loginTime. It uses the stored cookie format that Login writes.
	userSessionCookie := func() *http.Cookie {
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

	// The handler stands in for a hook or a queued plugin task: it reports
	// who the request runs as, makes the plugin cookie for it and keeps the
	// request context, which a queued task uses when it starts later.
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

	loginTimeOf := func(c *http.Cookie) time.Time {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(c)
		sess, err := sessions.GetSession(httptest.NewRecorder(), r)
		require.NoError(t, err)
		require.NotNil(t, sess)
		return sess.LoginTime
	}

	t.Run("session cookie", func(t *testing.T) {
		users.admin.UpdatedAt = notUpdatedSinceLogin

		w := request("", userSessionCookie())
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "admin", w.Body.String())
		require.NotNil(t, pluginCookie)
		got := loginTimeOf(pluginCookie)
		require.True(t, got.Equal(loginTime),
			"plugin cookie login time = %v, want the session's %v", got, loginTime)

		cb := request("", pluginCookie)
		require.Equal(t, http.StatusOK, cb.Code)
		require.Equal(t, "admin", cb.Body.String())
	})

	t.Run("api key", func(t *testing.T) {
		users.admin.UpdatedAt = notUpdatedSinceLogin

		before := time.Now()
		w := request("admin-key", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "admin", w.Body.String())
		require.NotNil(t, pluginCookie)
		lt := loginTimeOf(pluginCookie)
		require.False(t, lt.Before(before), "login time %v is before the request %v", lt, before)
		require.False(t, lt.After(time.Now()))

		cb := request("", pluginCookie)
		require.Equal(t, http.StatusOK, cb.Code)
		require.Equal(t, "admin", cb.Body.String())
	})

	t.Run("no user", func(t *testing.T) {
		w := request("", nil)
		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Nil(t, pluginCookie, "the handler is not reached without a user")

		c := sessions.MakePluginCookie(context.Background())
		require.NotNil(t, c)
		cb := request("", c)
		require.Equal(t, http.StatusUnauthorized, cb.Code)
	})

	t.Run("revoked session stays revoked", func(t *testing.T) {
		users.admin.UpdatedAt = notUpdatedSinceLogin
		userCookie := userSessionCookie()

		w := request("", userCookie)
		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, pluginCookie)
		pc := pluginCookie

		// revoke: the user changes (for example a password change) after
		// the session logged in, before the plugin calls back
		users.admin.UpdatedAt = updatedSinceLogin

		require.Equal(t, http.StatusUnauthorized, request("", userCookie).Code)
		require.Nil(t, pluginCookie, "a revoked session can't start a plugin")
		require.Equal(t, http.StatusUnauthorized, request("", pc).Code,
			"plugin cookie must be refused once the user's session is revoked")
	})

	t.Run("queued task started after revocation", func(t *testing.T) {
		users.admin.UpdatedAt = notUpdatedSinceLogin

		// the session queues a task; the task keeps the request context
		w := request("", userSessionCookie())
		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, requestCtx)
		queuedCtx := requestCtx

		// the session is revoked while the task waits in the queue
		users.admin.UpdatedAt = updatedSinceLogin

		// the task starts and makes its plugin cookie now
		c := sessions.MakePluginCookie(queuedCtx)
		require.NotNil(t, c)
		require.True(t, loginTimeOf(c).Equal(loginTime))
		require.Equal(t, http.StatusUnauthorized, request("", c).Code,
			"a task queued by a revoked session must not call back")
	})
}
