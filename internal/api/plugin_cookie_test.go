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

func (s *pluginCookieUsers) ValidateCredentials(_ context.Context, username, password string) error {
	if username == "admin" && password == "admin-password" {
		return nil
	}
	return user.ErrAccessDenied
}

// TestPluginCookieAuthenticatesCallback checks that the cookie handed to a
// plugin lets the plugin call back as the same user, like upstream, while a
// revoked session stays revoked.
func TestPluginCookieAuthenticatesCallback(t *testing.T) {
	cfg := config.InitializeEmpty()
	cfg.SetPublicAccess(true)
	cfg.SetString(config.SessionStoreKey, "plugin-cookie-test-cookie-secret")
	cfg.SetInt(config.MaxSessionAge, 3600)
	db := mocks.NewDatabase()
	tx := db.Repository().TxnManager

	users := &pluginCookieUsers{admin: &models.User{
		ID:        1,
		Username:  "admin",
		Roles:     models.Roles{models.RoleEnumAdmin},
		UpdatedAt: time.Now().Add(-time.Hour),
	}}
	sessions := session.NewStore(cfg, users, &tx)

	// The handler stands in for a plugin hook: it reports who the request
	// runs as and makes the plugin cookie for it.
	var pluginCookie *http.Cookie
	h := authenticateHandlerWithServices(tx, cfg, func() UserAuthenticator { return users }, sessions)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			name := "none"
			if u := session.GetCurrentUser(r.Context()); u != nil {
				name = u.Username
			}
			pluginCookie = sessions.MakePluginCookie(r.Context())
			fmt.Fprint(w, name)
		}))

	request := func(key string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		pluginCookie = nil
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

	login := func() *http.Cookie {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=admin-password"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		require.NoError(t, sessions.Login(w, r))
		cookies := w.Result().Cookies()
		require.Len(t, cookies, 1)
		return cookies[0]
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
		userCookie := login()

		w := request("", userCookie)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "admin", w.Body.String())
		require.NotNil(t, pluginCookie)
		require.True(t, loginTimeOf(userCookie).Equal(loginTimeOf(pluginCookie)),
			"plugin cookie must carry the user's session login time")

		cb := request("", pluginCookie)
		require.Equal(t, http.StatusOK, cb.Code)
		require.Equal(t, "admin", cb.Body.String())
	})

	t.Run("api key", func(t *testing.T) {
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
		userCookie := login()

		w := request("", userCookie)
		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, pluginCookie)
		pc := pluginCookie

		// revoke: the user changes (for example a password change) after
		// the session logged in, before the plugin calls back
		time.Sleep(2 * time.Millisecond)
		users.admin.UpdatedAt = time.Now()

		require.Equal(t, http.StatusUnauthorized, request("", userCookie).Code)
		require.Nil(t, pluginCookie, "a revoked session can't start a plugin")
		require.Equal(t, http.StatusUnauthorized, request("", pc).Code,
			"plugin cookie must be refused once the user's session is revoked")
	})
}
