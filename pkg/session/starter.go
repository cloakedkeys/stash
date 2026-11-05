package session

import (
	"context"
	"time"

	"github.com/stashapp/stash/pkg/models"
)

// PluginStarter is who started a plugin: the user of the request, and what a
// plugin cookie made for that request would carry, so that a plugin running
// in-process can be checked the same way a returning cookie is.
type PluginStarter struct {
	User models.User
	// LoginTime is the login time of the session that signed in the request.
	// Zero if the request had no session (API key, signed URL, single user
	// mode), as for MakePluginCookie, which then uses the time the plugin
	// starts.
	LoginTime time.Time
	// APIKeyFingerprint is set if the request was signed in with an API key.
	APIKeyFingerprint string
}

// GetPluginStarter returns who started a plugin from ctx, or nil if ctx has no
// current user.
func GetPluginStarter(ctx context.Context) *PluginStarter {
	u := GetCurrentUser(ctx)
	if u == nil {
		return nil
	}

	ret := &PluginStarter{User: *u}
	ret.LoginTime, _ = getSessionLoginTime(ctx)
	ret.APIKeyFingerprint, _ = getAPIKeyFingerprint(ctx)
	return ret
}

// PluginContext returns a new context for the calls of a plugin that s started.
// It carries the user, a login time and the API key fingerprint, the same
// things a plugin cookie carries, so that the calls are refused once the
// session is revoked or the key is deleted, and so that cookies for hooks the
// plugin starts carry them too. Call it once, when the plugin starts.
func (s *PluginStarter) PluginContext() context.Context {
	ctx := context.Background()
	if s == nil {
		return ctx
	}

	ctx = SetCurrentUser(ctx, s.User)

	loginTime := s.LoginTime
	if loginTime.IsZero() {
		// as MakePluginCookie: a request without a session was checked just now
		loginTime = time.Now()
	}
	ctx = SetSessionLoginTime(ctx, loginTime)

	if s.APIKeyFingerprint != "" {
		ctx = SetAPIKeyFingerprint(ctx, s.APIKeyFingerprint)
	}

	return ctx
}

// GetPluginCallAuth returns the login time and API key fingerprint carried by
// the context of a plugin's in-process call. ok is false if there is no
// login time.
func GetPluginCallAuth(ctx context.Context) (loginTime time.Time, apiKeyFingerprint string, ok bool) {
	loginTime, ok = getSessionLoginTime(ctx)
	apiKeyFingerprint, _ = getAPIKeyFingerprint(ctx)
	return loginTime, apiKeyFingerprint, ok
}
