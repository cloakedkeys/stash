package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/api/loaders"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/signedurl"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stashapp/stash/pkg/user"
)

func allowUnauthenticated(r *http.Request) bool {
	// #2715 - allow access to UI files
	return strings.HasPrefix(r.URL.Path, loginEndpoint) || r.URL.Path == logoutEndpoint || r.URL.Path == "/css" || strings.HasPrefix(r.URL.Path, "/assets")
}

func handleUnauthorized(w http.ResponseWriter, r *http.Request) {
	// if graphql or a non-webpage was requested, we just return a forbidden error
	ext := path.Ext(r.URL.Path)
	if r.URL.Path == gqlEndpoint || (ext != "" && ext != ".html") {
		w.Header().Add("WWW-Authenticate", "FormBased")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	prefix := getProxyPrefix(r)

	returnURL := url.URL{
		Path:     prefix + r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}
	q := make(url.Values)
	q.Set(returnURLParam, returnURL.String())
	u := url.URL{
		Path:     prefix + loginEndpoint,
		RawQuery: q.Encode(),
	}
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// authenticateSignedRequest checks if the request is a valid signed media request.
// Returns the matched user, or nil if the request is not a valid signed request.
func authenticateSignedRequest(ctx context.Context, r *http.Request, txnMgr models.TxnManager, g UserAuthenticator) (*models.User, error) {
	// Only apply to scene stream paths (used by AirPlay/Chromecast devices that can't pass cookies)
	if !strings.HasPrefix(r.URL.Path, "/scene/") && !strings.HasPrefix(r.URL.Path, "/audio/") {
		return nil, nil
	}

	// Check for signed URL parameters
	q := r.URL.Query()
	if q.Get(signedurl.CIDParam) == "" || q.Get(signedurl.ExpiresParam) == "" || q.Get(signedurl.SigParam) == "" {
		return nil, nil
	}

	var users []*models.User
	if err := txn.WithReadTxn(ctx, txnMgr, func(ctx context.Context) error {
		var err error
		users, err = g.AllUsers(ctx)
		return err
	}); err != nil {
		return nil, fmt.Errorf("error getting users: %w", err)
	}

	// Extract the credential ID and look up the user's signing key.
	// We need the key before we can verify the signature, since
	// each user could have their own signing key.
	c := config.GetInstance()
	u, secret, found := resolveCredentialID(c, users, q.Get(signedurl.CIDParam))
	if !found {
		logger.Warnf("signed URL credential ID mismatch")
		return nil, nil
	}

	// Verify the signature using the user's signing key
	if _, err := signedurl.VerifyURL(r.URL.Path, q, secret); err != nil {
		logger.Warnf("signed URL verification failed: %v", err)
		return nil, nil
	}

	return u, nil
}

type UserAuthenticator interface {
	AllUsers(ctx context.Context) ([]*models.User, error)
	GetGuestUser(ctx context.Context) *models.User

	IsSingleUserMode() bool
	GetSingleUser(ctx context.Context) (*models.User, error)

	AuthenticateByAPIKey(ctx context.Context, apiKey string) (*models.User, error)
	AuthenticateSession(ctx context.Context, username string, loginTime time.Time) (*models.User, error)
}

func httpError(w http.ResponseWriter, r *http.Request, text string, status int) {
	// if request accepts json, return json error response
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"error": "%s"}`, text)
	} else {
		http.Error(w, text, status)
	}
}

type PublicIPWhitelistGetter interface {
	GetPublicWhitelist() ([]net.IPNet, []net.IP)
}

func matchIPWhitelist(g PublicIPWhitelistGetter, requestIP net.IP) bool {
	nets, addrs := g.GetPublicWhitelist()

	for _, addr := range addrs {
		if addr.Equal(requestIP) {
			return true
		}
	}
	for _, net := range nets {
		if net.Contains(requestIP) {
			return true
		}
	}
	return false
}

type authenticationConfig interface {
	IsNewSystem() bool
	GetNewSystemCredentials() (string, string, time.Time)

	GetPublicAccess() bool
	PublicIPWhitelistGetter
}

func authenticateHandler(txnMgr models.TxnManager, cfg authenticationConfig) func(http.Handler) http.Handler {
	return authenticateHandlerWithServices(txnMgr, cfg, func() UserAuthenticator {
		return manager.GetInstance().UserService
	}, manager.GetInstance().SessionStore)
}

func authenticateHandlerWithServices(txnMgr models.TxnManager, cfg authenticationConfig, getUsers func() UserAuthenticator, sessions *session.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			g := getUsers()
			singleUserMode := g.IsSingleUserMode()
			publicAccess := cfg.GetPublicAccess()

			if singleUserMode && publicAccess {
				// mis-configuration - cannot enable single user mode and public access at the same time
				httpError(w, r, "Server misconfiguration: single user mode and public access cannot both be enabled", http.StatusServiceUnavailable)
				return
			}

			if !publicAccess {
				requestIP, err := getRequestIPFromCtx(ctx)
				if err != nil {
					logger.Errorf("error getting request IP: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				if !isLocalIP(requestIP) && !matchIPWhitelist(cfg, requestIP) {
					httpError(w, r, "Access denied: Stash is not configured to allow access from external IPs", http.StatusForbidden)
					return
				}
			}

			if cfg.IsNewSystem() {
				expectedUsername, _, startupTime := cfg.GetNewSystemCredentials()

				session, _ := sessions.GetSession(w, r)
				if !allowUnauthenticated(r) && expectedUsername != "" && (session == nil || session.UserID != expectedUsername || session.LoginTime.Before(startupTime)) {
					handleUnauthorized(w, r)
					return
				}

				next.ServeHTTP(w, r)
				return
			}

			var u *models.User
			var err error
			// login time of the session cookie that authenticated u, if any
			var sessionLoginTime time.Time

			if singleUserMode {
				u, err = singleUser(ctx, txnMgr, g)
				if err != nil {
					logger.Error(err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			}

			r = session.SetLocalRequest(r)

			if singleUserMode {
				if apiKey := session.GetRequestApiKey(r); apiKey != "" {
					// Check only for the origin exemption; keep using the default user.
					if err := txn.WithReadTxn(ctx, txnMgr, func(ctx context.Context) error {
						_, err := g.AuthenticateByAPIKey(ctx, apiKey)
						return err
					}); err != nil {
						logger.Errorf("error checking API key for origin exemption: %v", err)
					} else {
						ctx = context.WithValue(ctx, authenticatedAPIKeyCtxKey, true)
					}
				}
			}

			if !singleUserMode {
				// try to authenticate using api key first
				apiKey := session.GetRequestApiKey(r)
				if apiKey != "" {
					requestIP, err := keyByIP(r)
					if err != nil {
						logger.Errorf("Error getting IP for API key rate limiting: %v", err)
						httpError(w, r, "An unexpected error occurred. See logs", http.StatusInternalServerError)
						return
					}

					if loginFailureLimiter.isLimited(requestIP) {
						httpError(w, r, "Too many attempts using bad API key. Please try again later.", http.StatusTooManyRequests)
						return
					}

					if err := txn.WithReadTxn(ctx, txnMgr, func(ctx context.Context) error {
						u, err = g.AuthenticateByAPIKey(ctx, apiKey)
						return err
					}); err != nil {
						if errors.Is(err, user.ErrInternalError) {
							logger.Errorf("error authenticating by API key: %v", err)
							httpError(w, r, "internal server error", http.StatusInternalServerError)
							return
						}

						// must be authentication failure
						// rate limit failed API key attempts to prevent brute forcing
						loginFailureLimiter.recordFailure(requestIP)

						// don't forward to login page if api key auth fails
						httpError(w, r, "Invalid API key", http.StatusUnauthorized)
						return
					}

					// Only a successfully checked key exempts a request from origin checks.
					ctx = context.WithValue(ctx, authenticatedAPIKeyCtxKey, true)
					// plugin cookies made for this request only work while the key does
					ctx = session.SetAPIKeyFingerprint(ctx, session.APIKeyFingerprint(u.ApiKeyHash))
				} else if signedUser, signedErr := authenticateSignedRequest(ctx, r, txnMgr, g); signedErr != nil {
					logger.Errorf("error authenticating signed request: %v", signedErr)
					httpError(w, r, "internal server error", http.StatusInternalServerError)
					return
				} else if signedUser != nil {
					u = signedUser
				} else {
					sess, getErr := sessions.GetSession(w, r)
					if getErr != nil {
						logger.Errorf("error getting session user ID: %v", getErr)
						httpError(w, r, "internal server error", http.StatusInternalServerError)
						return
					}

					if sess != nil {
						var authErr error
						u, authErr = authenticateSession(ctx, txnMgr, g, sess.UserID, sess.LoginTime, sess.APIKeyFingerprint)
						if authErr != nil {
							logger.Errorf("error authenticating user by ID: %v", authErr)
							httpError(w, r, "internal server error", http.StatusInternalServerError)
							return
						}
						if u != nil {
							sessionLoginTime = sess.LoginTime
							if sess.APIKeyFingerprint != "" {
								// plugin cookies made for this request stay tied to the key
								ctx = session.SetAPIKeyFingerprint(ctx, sess.APIKeyFingerprint)
							}
						}
					}
				}

				if err != nil {
					if errors.Is(err, user.ErrInternalError) {
						httpError(w, r, "internal server error", http.StatusInternalServerError)
						return
					}

					// fall through and treat as unauthenticated
				}
			}

			if u == nil && !singleUserMode {
				u = g.GetGuestUser(ctx)
			}

			// authentication is required
			if u == nil && !allowUnauthenticated(r) {
				handleUnauthorized(w, r)
				return
			}

			if u != nil {
				// set the user object in the context
				ctx = session.SetCurrentUser(ctx, *u)
				if !sessionLoginTime.IsZero() {
					// plugin cookies made for this request keep the session's login time
					ctx = session.SetSessionLoginTime(ctx, sessionLoginTime)
				}
			}

			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

// authenticateSession checks the user of a session: the user must exist, not
// be locked and not have changed since loginTime. If the session was made from
// an API key request (apiKeyFingerprint is set), the user must still have that
// key. It returns nil if the session is refused, and an error only for an
// internal error.
func authenticateSession(ctx context.Context, txnMgr models.TxnManager, g UserAuthenticator, username string, loginTime time.Time, apiKeyFingerprint string) (*models.User, error) {
	var u *models.User
	if err := txn.WithReadTxn(ctx, txnMgr, func(ctx context.Context) error {
		var err error
		u, err = g.AuthenticateSession(ctx, username, loginTime)
		return err
	}); err != nil {
		if errors.Is(err, user.ErrInternalError) {
			return nil, err
		}
		return nil, nil
	}

	if u != nil && apiKeyFingerprint != "" && !session.MatchesAPIKeyFingerprint(apiKeyFingerprint, u.ApiKeyHash) {
		// a plugin session made from an API key request, and the key has
		// since been deleted or replaced
		logger.Infof("[authentication] API key of user %s has changed since the plugin session started", u.Username)
		return nil, nil
	}

	return u, nil
}

// checkPluginCaller checks the user that an in-process plugin call acts as,
// the same way a returning plugin cookie is checked. It returns the user as
// stored now, or nil if the call is refused.
func checkPluginCaller(ctx context.Context, txnMgr models.TxnManager, g UserAuthenticator, username string) (*models.User, error) {
	loginTime, apiKeyFingerprint, ok := session.GetPluginCallAuth(ctx)
	if !ok {
		// the plugin context always carries a login time; without one the
		// call can't be checked
		logger.Infof("[authentication] plugin call for user %s has no login time", username)
		return nil, nil
	}

	return authenticateSession(ctx, txnMgr, g, username, loginTime, apiKeyFingerprint)
}

// singleUser returns the single user, with the admin role, that every request
// acts as in single-user mode. It returns nil if there is no such user.
func singleUser(ctx context.Context, txnMgr models.TxnManager, g UserAuthenticator) (*models.User, error) {
	var u *models.User
	if err := txn.WithReadTxn(ctx, txnMgr, func(ctx context.Context) error {
		var err error
		u, err = g.GetSingleUser(ctx)
		if err != nil {
			return fmt.Errorf("error retrieving default user: %w", err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	if u != nil {
		u.Roles = models.Roles{models.RoleEnumAdmin}
	}

	return u, nil
}

// pluginGQLHandler is the GraphQL handler that JavaScript plugins call
// in-process. It sets the user first, as the dataloaders apply the user's
// library limit, then reads the visited plugin hooks from the plugin's cookie.
func pluginGQLHandler(gql http.Handler, txnMgr models.TxnManager, getUsers func() UserAuthenticator, visitedPlugins func(http.Handler) http.Handler, dataloaders loaders.Middleware) http.Handler {
	return pluginUserHandler(txnMgr, getUsers)(visitedPlugins(dataloaders.Middleware(gql)))
}

// pluginUserHandler sets the current user for the GraphQL requests that
// JavaScript plugins make in-process. Those requests don't pass through
// authenticateHandler, so the plugin brings the user whose request started it
// in the request context. In single-user mode every request acts as the single
// user, as authenticateHandler does. In multi-user mode each call is checked
// as a returning plugin cookie is, and a plugin that nobody started is treated
// as an unauthenticated request: it acts as the guest account if there is one,
// and is refused otherwise.
func pluginUserHandler(txnMgr models.TxnManager, getUsers func() UserAuthenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			g := getUsers()
			if g.IsSingleUserMode() {
				u, err := singleUser(r.Context(), txnMgr, g)
				if err != nil {
					logger.Error(err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				if u != nil {
					r = r.WithContext(session.SetCurrentUser(r.Context(), *u))
				}
			} else {
				var u *models.User
				if starter := session.GetCurrentUser(r.Context()); starter != nil {
					var err error
					u, err = checkPluginCaller(r.Context(), txnMgr, g, starter.Username)
					if err != nil {
						logger.Errorf("error checking plugin user: %v", err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
				}

				if u == nil {
					// Nobody started the plugin, or its user was refused, as a
					// returning plugin cookie can be. It acts as an
					// unauthenticated request does: as the guest account if
					// there is one, and refused otherwise. The role checks let a
					// call with no user through, which is only meant for a new
					// system that has no accounts yet.
					u = g.GetGuestUser(r.Context())
				}
				if u == nil {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				// the user as stored now, so role and library limit changes apply
				r = r.WithContext(session.SetCurrentUser(r.Context(), *u))
			}

			next.ServeHTTP(w, r)
		})
	}
}
