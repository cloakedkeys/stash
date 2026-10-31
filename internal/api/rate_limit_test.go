package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stashapp/stash/pkg/user"
	"github.com/stretchr/testify/assert"
)

func TestFailureLimiter(t *testing.T) {
	const limit = 3
	l := newFailureLimiter(limit, time.Minute)
	const key = "key"

	for i := 0; i < limit; i++ {
		assert.False(t, l.isLimited(key))
		assert.True(t, l.reserve(key), "attempt %d", i)
		l.finish(key, true)
	}

	assert.True(t, l.isLimited(key))
	assert.False(t, l.reserve(key))
	assert.True(t, l.reserve("other"), "other keys aren't affected")
}

func TestFailureLimiter_SuccessDoesNotCount(t *testing.T) {
	l := newFailureLimiter(2, time.Minute)

	for i := 0; i < 10; i++ {
		assert.True(t, l.reserve("key"), "attempt %d", i)
		l.finish("key", false)
	}
}

func TestFailureLimiter_ConcurrentAttemptsCount(t *testing.T) {
	l := newFailureLimiter(2, time.Minute)

	assert.True(t, l.reserve("key"))
	assert.True(t, l.reserve("key"))
	assert.False(t, l.reserve("key"), "attempts being checked use up the limit")

	l.finish("key", false)
	assert.True(t, l.reserve("key"), "a finished success gives its place back")
}

func TestFailureLimiter_Expires(t *testing.T) {
	l := newFailureLimiter(1, 10*time.Millisecond)

	l.recordFailure("key")
	assert.True(t, l.isLimited("key"))

	time.Sleep(20 * time.Millisecond)
	assert.False(t, l.isLimited("key"))

	// the next call sweeps the expired key away
	l.recordFailure("other")
	assert.NotContains(t, l.entries, "key")
}

type loginTestConfig struct{ authenticationConfig }

func (loginTestConfig) IsNewSystem() bool { return false }

type loginTestSessionConfig struct{}

func (loginTestSessionConfig) GetSessionStoreKey() []byte { return []byte(strings.Repeat("k", 32)) }
func (loginTestSessionConfig) GetMaxSessionAge() int      { return 3600 }

type loginTestTxnReader struct{}

func (loginTestTxnReader) WithReadTxn(ctx context.Context, fn txn.TxnFunc) error { return fn(ctx) }

type loginTestAuthenticator struct{}

func (loginTestAuthenticator) ValidateCredentials(_ context.Context, _, password string) error {
	if password == "right" {
		return nil
	}
	return user.ErrAccessDenied
}

func TestHandleLoginPost_RateLimit(t *testing.T) {
	loginFailureLimiter = newFailureLimiter(loginFailureLimit, loginFailureWindow)
	userLoginFailureLimiter = newFailureLimiter(userLoginFailureLimit, userLoginFailureWindow)

	store := session.NewStore(loginTestSessionConfig{}, loginTestAuthenticator{}, loginTestTxnReader{})
	handler := handleLoginPost(store, loginTestConfig{})

	login := func(ip, username, password string) int {
		form := url.Values{"username": {username}, "password": {password}}
		r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r = r.WithContext(context.WithValue(r.Context(), IPCtxKey, net.ParseIP(ip)))
		w := httptest.NewRecorder()
		handler(w, r)
		return w.Code
	}

	const victim = "victim"
	const victimIP = "10.0.0.1"
	const attackerIP = "10.0.0.2"

	// successful logins never use up the limit
	for i := 0; i < loginFailureLimit*2; i++ {
		assert.Equal(t, http.StatusOK, login(victimIP, victim, "right"), "login %d", i)
	}

	// one source using up its limit on someone's username stops only that source
	for i := 0; i < loginFailureLimit; i++ {
		assert.Equal(t, http.StatusUnauthorized, login(attackerIP, victim, "wrong"), "guess %d", i)
	}
	assert.Equal(t, http.StatusTooManyRequests, login(attackerIP, victim, "wrong"))
	assert.Equal(t, http.StatusTooManyRequests, login(attackerIP, victim, "right"))
	assert.Equal(t, http.StatusOK, login(victimIP, victim, "right"))

	// guessing from many sources is still limited
	loginFailureLimiter = newFailureLimiter(userLoginFailureLimit*2, loginFailureWindow)
	for i := loginFailureLimit; i < userLoginFailureLimit; i++ {
		assert.Equal(t, http.StatusUnauthorized, login(attackerIP, victim, "wrong"), "guess %d", i)
	}
	assert.Equal(t, http.StatusTooManyRequests, login("10.0.0.3", victim, "wrong"))
	assert.Equal(t, http.StatusTooManyRequests, login("10.0.0.3", strings.ToUpper(victim), "wrong"), "case doesn't avoid the limit")
}
