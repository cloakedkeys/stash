// Package session provides session authentication and management for the application.
package session

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/sessions"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
)

type key int

const (
	contextUser key = iota
	contextVisitedPlugins
	contextLocalRequest
	contextSessionLoginTime
	contextAPIKeyFingerprint
)

const (
	userIDKey             = "userID"
	loginTimeKey          = "loginTime"
	loginTimeNanosKey     = "loginTimeNanos"
	visitedPluginHooksKey = "visitedPluginsHooks"
	apiKeyFingerprintKey  = "apiKey"
)

const (
	ApiKeyHeader    = "ApiKey"
	ApiKeyParameter = "apikey"
)

const (
	cookieName      = "session"
	usernameFormKey = "username"
	passwordFormKey = "password"
)

type InvalidCredentialsError struct {
	Username string
}

func (e InvalidCredentialsError) Error() string {
	// don't leak the username
	return "invalid credentials"
}

var ErrUnauthorized = errors.New("unauthorized")

type Authenticator interface {
	ValidateCredentials(ctx context.Context, username string, password string) error
}

type TxnReader interface {
	WithReadTxn(ctx context.Context, fn txn.TxnFunc) error
}

type Store struct {
	sessionStore *sessions.CookieStore

	authMutex     sync.RWMutex
	authenticator Authenticator

	txnReader TxnReader
	config    SessionConfig
}

func NewStore(c SessionConfig, a Authenticator, txnReader TxnReader) *Store {
	ret := &Store{
		sessionStore:  sessions.NewCookieStore(c.GetSessionStoreKey()),
		txnReader:     txnReader,
		config:        c,
		authenticator: a,
	}

	ret.sessionStore.MaxAge(c.GetMaxSessionAge())
	ret.sessionStore.Options.SameSite = http.SameSiteLaxMode

	return ret
}

func GetUsernameFromForm(r *http.Request) string {
	return r.FormValue(usernameFormKey)
}

func (s *Store) RegisterAuthenticator(a Authenticator) {
	s.authMutex.Lock()
	defer s.authMutex.Unlock()
	s.authenticator = a
}

func (s *Store) getAuthenticator() Authenticator {
	s.authMutex.RLock()
	defer s.authMutex.RUnlock()
	return s.authenticator
}

func (s *Store) Login(w http.ResponseWriter, r *http.Request) error {
	// ignore error - we want a new session regardless
	newSession, _ := s.sessionStore.Get(r, cookieName)

	username := r.FormValue(usernameFormKey)
	password := r.FormValue(passwordFormKey)

	// authenticate the user
	if err := s.txnReader.WithReadTxn(r.Context(), func(ctx context.Context) error {
		return s.getAuthenticator().ValidateCredentials(ctx, username, password)
	}); err != nil {
		return &InvalidCredentialsError{Username: username}
	}

	logger.Infof("User %s logged in", username)

	newSession.Values[userIDKey] = username
	loginTime := time.Now()
	newSession.Values[loginTimeKey] = loginTime.Unix()
	newSession.Values[loginTimeNanosKey] = loginTime.UnixNano()

	err := newSession.Save(r, w)
	if err != nil {
		return err
	}

	return nil
}

// LoginForSetup handles login for the initial setup of a new system. It checks the provided credentials against the expected credentials for a new system setup, which are stored in the config. This is separate from the regular Login function to handle the case where there is no session store or user service available yet.
func (s *Store) LoginForSetup(w http.ResponseWriter, r *http.Request, expectedUser, expectedPassword string) error {
	// ignore error - we want a new session regardless
	newSession, _ := s.sessionStore.Get(r, cookieName)

	username := r.FormValue(usernameFormKey)
	password := r.FormValue(passwordFormKey)

	// authenticate the user
	if username != expectedUser || password != expectedPassword {
		return &InvalidCredentialsError{Username: username}
	}

	logger.Info("Setup user logged in")

	newSession.Values[userIDKey] = username
	loginTime := time.Now()
	newSession.Values[loginTimeKey] = loginTime.Unix()
	newSession.Values[loginTimeNanosKey] = loginTime.UnixNano()

	err := newSession.Save(r, w)
	if err != nil {
		return err
	}

	return nil
}

func (s *Store) Logout(w http.ResponseWriter, r *http.Request) error {
	session, err := s.sessionStore.Get(r, cookieName)
	if err != nil {
		return err
	}

	userID, _ := session.Values[userIDKey].(string)

	delete(session.Values, userIDKey)
	session.Options.MaxAge = -1

	err = session.Save(r, w)
	if err != nil {
		return err
	}

	logger.Infof("User %s logged out", userID)

	return nil
}

type Session struct {
	UserID    string
	LoginTime time.Time
	// APIKeyFingerprint is set on plugin cookies made for a request that was
	// signed in with an API key. The cookie is only valid while the user still
	// has that key. Empty for a normal login.
	APIKeyFingerprint string
}

func (s *Store) GetSession(w http.ResponseWriter, r *http.Request) (*Session, error) {
	session, err := s.sessionStore.Get(r, cookieName)
	// ignore errors and treat as an empty user id, so that we handle expired
	// cookie
	if err != nil {
		return nil, nil
	}

	if !session.IsNew {
		// refresh the cookie
		err = session.Save(r, w)
		if err != nil {
			return nil, err
		}

		ret := &Session{}
		ret.UserID, _ = session.Values[userIDKey].(string)
		loginTimeUnix, _ := session.Values[loginTimeKey].(int64)
		ret.LoginTime = time.Unix(loginTimeUnix, 0)
		if loginTimeNanos, ok := session.Values[loginTimeNanosKey].(int64); ok {
			ret.LoginTime = time.Unix(0, loginTimeNanos)
		}
		ret.APIKeyFingerprint, _ = session.Values[apiKeyFingerprintKey].(string)

		return ret, nil
	}

	return nil, nil
}

func SetCurrentUser(ctx context.Context, u models.User) context.Context {
	ctx = models.WithUserID(ctx, u.ID)
	ctx = models.WithLibraryLimit(ctx, u.LibraryLimit)
	return context.WithValue(ctx, contextUser, u)
}

// SetSessionLoginTime records the login time of the session cookie that
// authenticated the current request. Plugin cookies made for the request
// carry it, so they are revoked together with the user's own session.
func SetSessionLoginTime(ctx context.Context, loginTime time.Time) context.Context {
	return context.WithValue(ctx, contextSessionLoginTime, loginTime)
}

func getSessionLoginTime(ctx context.Context) (time.Time, bool) {
	t, ok := ctx.Value(contextSessionLoginTime).(time.Time)
	return t, ok
}

// APIKeyFingerprint identifies the API key whose stored hash is apiKeyHash,
// without putting the hash itself in a cookie. A new key, or no key, gives a
// different fingerprint.
func APIKeyFingerprint(apiKeyHash string) string {
	sum := sha256.Sum256([]byte("stash plugin cookie api key:" + apiKeyHash))
	return hex.EncodeToString(sum[:])
}

// MatchesAPIKeyFingerprint reports whether fingerprint identifies the API key
// whose stored hash is apiKeyHash.
func MatchesAPIKeyFingerprint(fingerprint, apiKeyHash string) bool {
	want := APIKeyFingerprint(apiKeyHash)
	return subtle.ConstantTimeCompare([]byte(fingerprint), []byte(want)) == 1
}

// SetAPIKeyFingerprint records that the current request was signed in with the
// API key that has this fingerprint. Plugin cookies made for the request carry
// it, so they stop working once the user's key is deleted or replaced.
func SetAPIKeyFingerprint(ctx context.Context, fingerprint string) context.Context {
	return context.WithValue(ctx, contextAPIKeyFingerprint, fingerprint)
}

func getAPIKeyFingerprint(ctx context.Context) (string, bool) {
	fp, ok := ctx.Value(contextAPIKeyFingerprint).(string)
	return fp, ok && fp != ""
}

// GetCurrentUser gets the current user id from the provided context
func GetCurrentUser(ctx context.Context) *models.User {
	userCtxVal := ctx.Value(contextUser)
	if userCtxVal != nil {
		currentUser := userCtxVal.(models.User)
		return &currentUser
	}

	return nil
}

func GetRequestApiKey(r *http.Request) string {
	apiKey := r.Header.Get(ApiKeyHeader)

	// try getting the api key as a query parameter
	if apiKey == "" {
		apiKey = r.URL.Query().Get(ApiKeyParameter)
	}

	return apiKey
}
