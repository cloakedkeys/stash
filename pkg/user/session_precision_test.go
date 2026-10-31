package user

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/require"
)

type precisionCookieConfig struct{}

func (precisionCookieConfig) GetSessionStoreKey() []byte { return []byte(strings.Repeat("k", 32)) }
func (precisionCookieConfig) GetMaxSessionAge() int      { return 3600 }

type precisionTxnReader struct{}

func (precisionTxnReader) WithReadTxn(ctx context.Context, fn txn.TxnFunc) error { return fn(ctx) }

func TestFreshLoginCookiePreservesRevocationPrecision(t *testing.T) {
	for _, setup := range []bool{false, true} {
		name := "login"
		if setup {
			name = "setup login"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			store := newMockStore()
			u := &models.User{ID: 1, Username: "reader", Roles: models.Roles{models.RoleEnumRead}, UpdatedAt: time.Now().Add(-time.Minute)}
			store.users[u.Username] = u
			hash, err := hashPassword("password")
			require.NoError(t, err)
			store.passwords[u.ID] = hash
			svc := &Service{Store: store, Config: &mockConfig{}}
			require.NoError(t, svc.Init(ctx))
			cookies := session.NewStore(precisionCookieConfig{}, svc, precisionTxnReader{})
			form := url.Values{"username": {"reader"}, "password": {"password"}}
			r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			before := time.Now()
			if setup {
				err = cookies.LoginForSetup(w, r, "reader", "password")
			} else {
				err = cookies.Login(w, r)
			}
			after := time.Now()
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodGet, "/graphql", nil)
			for _, cookie := range w.Result().Cookies() {
				request.AddCookie(cookie)
			}
			login, err := cookies.GetSession(httptest.NewRecorder(), request)
			require.NoError(t, err)
			require.NotNil(t, login)
			require.False(t, login.LoginTime.Before(before), "cookie must not round a new login down before service startup")
			require.False(t, login.LoginTime.After(after))
			require.NotZero(t, login.LoginTime.Nanosecond(), "cookie must retain subsecond login precision")
			_, err = svc.AuthenticateSession(ctx, login.UserID, login.LoginTime)
			require.NoError(t, err)
			restarted := &Service{Store: store, Config: &mockConfig{}}
			require.NoError(t, restarted.Init(ctx))
			_, err = restarted.AuthenticateSession(ctx, login.UserID, login.LoginTime)
			require.NoError(t, err, "a restart doesn't end a session")
			u.UpdatedAt = login.LoginTime.Add(time.Nanosecond)
			_, err = svc.AuthenticateSession(ctx, login.UserID, login.LoginTime)
			require.ErrorIs(t, err, ErrAccessDenied, "user changes revoke even within the same second")
			_, err = restarted.AuthenticateSession(ctx, login.UserID, login.LoginTime)
			require.ErrorIs(t, err, ErrAccessDenied, "user changes still revoke after a restart")
		})
	}
}
