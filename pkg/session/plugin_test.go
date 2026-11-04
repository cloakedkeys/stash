package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/plugin/hook"
)

type testSessionConfig struct{}

func (testSessionConfig) GetSessionStoreKey() []byte {
	return []byte("0123456789abcdef0123456789abcdef")
}

func (testSessionConfig) GetMaxSessionAge() int {
	return 3600
}

func newTestStore() *Store {
	return NewStore(testSessionConfig{}, nil, nil)
}

// readPluginCookie sends the cookie back to the store the way a plugin does
// and returns the session it decodes to, plus the visited hooks the
// VisitedPluginHandler middleware puts in the request context.
func readPluginCookie(t *testing.T, s *Store, cookie *http.Cookie) (*Session, []VisitedPluginHook) {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	r.AddCookie(cookie)

	var visited []VisitedPluginHook
	s.VisitedPluginHandler()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visited = GetVisitedPluginHooks(r.Context())
	})).ServeHTTP(httptest.NewRecorder(), r)

	sess, err := s.GetSession(httptest.NewRecorder(), r)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	return sess, visited
}

func TestMakePluginCookieWithCurrentUser(t *testing.T) {
	s := newTestStore()

	ctx := SetCurrentUser(context.Background(), models.User{ID: 7, Username: "admin"})
	ctx = AddVisitedPluginHook(ctx, "plugin-a", hook.TagUpdatePost)

	cookie := s.MakePluginCookie(ctx)
	if cookie == nil {
		t.Fatal("MakePluginCookie returned nil for a request with a current user")
	}
	if cookie.Name != cookieName {
		t.Errorf("cookie name = %q, want %q", cookie.Name, cookieName)
	}

	sess, visited := readPluginCookie(t, s, cookie)
	if sess == nil {
		t.Fatal("plugin cookie did not decode to a session")
	}
	if sess.UserID != "admin" {
		t.Errorf("session user = %q, want %q", sess.UserID, "admin")
	}

	want := []VisitedPluginHook{{PluginID: "plugin-a", HookType: hook.TagUpdatePost}}
	if len(visited) != 1 || visited[0] != want[0] {
		t.Errorf("visited hooks = %v, want %v", visited, want)
	}
}

// A request authenticated by a session cookie: the plugin cookie keeps that
// session's login time, to the nanosecond.
func TestMakePluginCookieKeepsSessionLoginTime(t *testing.T) {
	s := newTestStore()

	loginTime := time.Unix(0, 1700000000123456789)
	ctx := SetCurrentUser(context.Background(), models.User{ID: 7, Username: "admin"})
	ctx = SetSessionLoginTime(ctx, loginTime)

	cookie := s.MakePluginCookie(ctx)
	if cookie == nil {
		t.Fatal("MakePluginCookie returned nil")
	}

	sess, _ := readPluginCookie(t, s, cookie)
	if sess == nil || sess.UserID != "admin" {
		t.Fatalf("session = %+v, want user admin", sess)
	}
	if !sess.LoginTime.Equal(loginTime) {
		t.Errorf("login time = %v, want %v", sess.LoginTime, loginTime)
	}
}

// A request with a user but no session (API key, signed URL, single user
// mode): the plugin cookie uses the time it was made.
func TestMakePluginCookieWithoutSessionUsesNow(t *testing.T) {
	s := newTestStore()

	ctx := SetCurrentUser(context.Background(), models.User{ID: 7, Username: "admin"})

	before := time.Now()
	cookie := s.MakePluginCookie(ctx)
	after := time.Now()
	if cookie == nil {
		t.Fatal("MakePluginCookie returned nil")
	}

	sess, _ := readPluginCookie(t, s, cookie)
	if sess == nil {
		t.Fatal("plugin cookie did not decode to a session")
	}
	if sess.LoginTime.Before(before) || sess.LoginTime.After(after) {
		t.Errorf("login time = %v, want between %v and %v", sess.LoginTime, before, after)
	}
}

func TestMakePluginCookieWithoutCurrentUser(t *testing.T) {
	s := newTestStore()

	ctx := AddVisitedPluginHook(context.Background(), "plugin-a", hook.SceneUpdatePost)
	ctx = AddVisitedPluginHook(ctx, "plugin-a", hook.SceneUpdatePost)

	cookie := s.MakePluginCookie(ctx)
	if cookie == nil {
		t.Fatal("MakePluginCookie returned nil")
	}

	sess, visited := readPluginCookie(t, s, cookie)
	if sess == nil {
		t.Fatal("plugin cookie did not decode to a session")
	}
	if sess.UserID != "" {
		t.Errorf("session user = %q, want empty", sess.UserID)
	}
	// no login time is stored, as before
	if !sess.LoginTime.Equal(time.Unix(0, 0)) {
		t.Errorf("login time = %v, want none", sess.LoginTime)
	}
	if len(visited) != 2 {
		t.Errorf("visited hooks = %v, want 2 entries", visited)
	}
}

// A browser login cookie still decodes the same way.
func TestLoginCookieStillDecodes(t *testing.T) {
	s := newTestStore()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	session, _ := s.sessionStore.Get(r, cookieName)
	session.Values[userIDKey] = "admin"
	session.Values[loginTimeKey] = int64(1700000000)
	session.Values[loginTimeNanosKey] = int64(1700000000123456789)

	w := httptest.NewRecorder()
	if err := session.Save(r, w); err != nil {
		t.Fatalf("saving session: %v", err)
	}

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}

	got, _ := readPluginCookie(t, s, cookies[0])
	if got == nil || got.UserID != "admin" {
		t.Fatalf("session = %+v, want user admin", got)
	}
	if got.LoginTime.UnixNano() != 1700000000123456789 {
		t.Errorf("login time = %d, want 1700000000123456789", got.LoginTime.UnixNano())
	}
}
