package api

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/httprate"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
)

type rateLimiter struct {
	WindowLength time.Duration
	RequestLimit int

	httprate.RateLimiter
}

func newRateLimiter(requestLimit int, windowLength time.Duration) *rateLimiter {
	rl := &rateLimiter{
		WindowLength: windowLength,
		RequestLimit: requestLimit,
	}

	rl.RateLimiter = *httprate.NewRateLimiter(requestLimit, windowLength)

	return rl
}

// failureEntry holds the attempts made using a key.
type failureEntry struct {
	failures []time.Time
	inFlight int
}

// failureLimiter limits the failed attempts made using a key within a window.
// Only failures count, so successful attempts never use up the limit. Attempts
// still being checked are reserved against the limit, so that sending many at
// once doesn't get more guesses than sending them one at a time.
type failureLimiter struct {
	limit  int
	window time.Duration

	mu        sync.Mutex
	entries   map[string]*failureEntry
	lastSweep time.Time
}

func newFailureLimiter(limit int, window time.Duration) *failureLimiter {
	return &failureLimiter{
		limit:   limit,
		window:  window,
		entries: make(map[string]*failureEntry),
	}
}

// entry returns the entry for the key without its expired failures, if it has one.
// assumes lock held
func (l *failureLimiter) entry(key string, now time.Time) *failureEntry {
	e := l.entries[key]
	if e != nil {
		cutoff := now.Add(-l.window)
		i := 0
		for i < len(e.failures) && !e.failures[i].After(cutoff) {
			i++
		}
		e.failures = e.failures[i:]
	}
	return e
}

// sweep removes entries with nothing left to count, at most once per window,
// so that keys that are never seen again don't remain forever.
// assumes lock held
func (l *failureLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now

	for key := range l.entries {
		if e := l.entry(key, now); len(e.failures) == 0 && e.inFlight == 0 {
			delete(l.entries, key)
		}
	}
}

// isLimited returns true if the key has reached the limit of failures.
func (l *failureLimiter) isLimited(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	e := l.entry(key, time.Now())
	return e != nil && len(e.failures) >= l.limit
}

// reserve starts an attempt using the key. It returns false if the limit doesn't
// allow one. Each successful call must be followed by a call to finish.
func (l *failureLimiter) reserve(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.sweep(now)

	e := l.entry(key, now)
	if e == nil {
		e = &failureEntry{}
		l.entries[key] = e
	}

	if len(e.failures)+e.inFlight >= l.limit {
		return false
	}

	e.inFlight++
	return true
}

// finish ends an attempt started with reserve.
func (l *failureLimiter) finish(key string, failed bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	e := l.entries[key]
	if e == nil {
		return
	}

	e.inFlight--
	if failed {
		e.failures = append(e.failures, time.Now())
	}
}

// recordFailure counts a failure that wasn't started with reserve.
func (l *failureLimiter) recordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.sweep(now)

	e := l.entry(key, now)
	if e == nil {
		e = &failureEntry{}
		l.entries[key] = e
	}
	e.failures = append(e.failures, now)
}

// TODO - this should probably be configurable
const (
	loginFailureLimit  = 5
	loginFailureWindow = 1 * time.Minute

	// A single source can't reach this limit because it's stopped by the limit
	// above first, so one source can't lock anyone out of their account. This
	// limit is for guessing a single account's password from many sources.
	userLoginFailureLimit  = 30
	userLoginFailureWindow = 1 * time.Minute
)

var (
	loginFailureLimiter     = newFailureLimiter(loginFailureLimit, loginFailureWindow)
	userLoginFailureLimiter = newFailureLimiter(userLoginFailureLimit, userLoginFailureWindow)
	userRateLimiter         *rateLimiter
)

// reserveLoginAttempt starts a login attempt from the IP using the username. It
// returns false if the attempt isn't allowed. Otherwise it returns the function
// to call with whether the attempt failed.
func reserveLoginAttempt(ip, username string) (func(failed bool), bool) {
	// usernames may differ only by case
	userKey := strings.ToLower(username)

	if !loginFailureLimiter.reserve(ip) {
		return nil, false
	}

	if !userLoginFailureLimiter.reserve(userKey) {
		loginFailureLimiter.finish(ip, false)
		return nil, false
	}

	return func(failed bool) {
		loginFailureLimiter.finish(ip, failed)
		userLoginFailureLimiter.finish(userKey, failed)
	}, true
}

// https://github.com/go-chi/httprate/issues/53
// KeyByRealIP is susceptible to spoof attacks
// We key by our trusted ip address instead.
func keyByIP(r *http.Request) (string, error) {
	ip, err := getRequestIPFromCtx(r.Context())
	if err != nil {
		return "", err
	}
	return ip.String(), nil
}

func rateLimitGraphql(rateLimit int, window time.Duration) func(http.Handler) http.Handler {
	if rateLimit <= 0 || window <= 0 {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	userRateLimiter = newRateLimiter(rateLimit, window)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, gqlEndpoint) {
				next.ServeHTTP(w, r)
				return
			}

			user := session.GetCurrentUser(r.Context())

			if user != nil && !user.Roles.HasRole(models.RoleEnumAdmin) {
				id := strconv.Itoa(user.ID)

				if userRateLimiter.OnLimit(w, r, id) {
					// don't log this as an error to prevent log spam, but do return a 429 to the user
					// TODO - really want to login this once per window
					httpError(w, r, "You have made too many requests. Please try again later.", http.StatusTooManyRequests)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
