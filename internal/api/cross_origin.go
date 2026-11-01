package api

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/stashapp/stash/pkg/logger"
)

var authenticatedAPIKeyCtxKey = &contextKey{"authenticatedAPIKey"}

type crossOriginProtection struct {
	protection *http.CrossOriginProtection
}

func newCrossOriginProtection(trustedOrigins []string) (*crossOriginProtection, error) {
	p := http.NewCrossOriginProtection()
	for _, origin := range trustedOrigins {
		if err := p.AddTrustedOrigin(origin); err != nil {
			return nil, fmt.Errorf("invalid trusted_origins entry: %w", err)
		}
	}
	return &crossOriginProtection{protection: p}, nil
}

func (p *crossOriginProtection) checkWebsocketOrigin(r *http.Request) bool {
	if r.Context().Value(authenticatedAPIKeyCtxKey) == true {
		return true
	}
	// Websocket handshakes use GET, but gqlgen also executes mutations over
	// this connection. Apply the standard library's unsafe-method check.
	check := new(http.Request)
	*check = *r
	check.Method = http.MethodPost
	return p.protection.Check(check) == nil
}

func (p *crossOriginProtection) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(authenticatedAPIKeyCtxKey) != true {
			allowed := p.protection.Check(r) == nil
			if websocket.IsWebSocketUpgrade(r) {
				allowed = p.checkWebsocketOrigin(r)
			}
			if !allowed {
				logger.Warnf("Refused cross-origin request from origin %q (Sec-Fetch-Site %q); add a trusted site to trusted_origins in config.yml and restart Stash", r.Header.Get("Origin"), r.Header.Get("Sec-Fetch-Site"))
				http.Error(w, "Cross-origin request refused. To allow a trusted site, add its origin to trusted_origins in config.yml and restart Stash.", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
