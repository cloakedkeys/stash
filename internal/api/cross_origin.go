package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/session"
)

const (
	castReceiverHTTPOrigin  = "http://www.gstatic.com"
	castReceiverHTTPSOrigin = "https://www.gstatic.com"
)

var authenticatedAPIKeyCtxKey = &contextKey{"authenticatedAPIKey"}

type crossOriginProtection struct {
	protection *http.CrossOriginProtection
}

func (p *crossOriginProtection) allowCORS(r *http.Request, origin string) bool {
	// The Cast receiver needs to read scene streams and captions without a key.
	method := r.Method
	if method == http.MethodOptions {
		method = r.Header.Get("Access-Control-Request-Method")
	}
	if (method == http.MethodGet || method == http.MethodHead) && strings.HasPrefix(r.URL.Path, "/scene/") &&
		(origin == castReceiverHTTPOrigin || origin == castReceiverHTTPSOrigin) {
		return true
	}
	if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
		// Preflights carry no key. Permission to send one does not grant
		// permission to read the actual response: authentication checks it first.
		for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
			if strings.EqualFold(strings.TrimSpace(header), session.ApiKeyHeader) {
				return true
			}
		}
	}
	if r.Context().Value(authenticatedAPIKeyCtxKey) == true {
		return true
	}
	// Check as an unsafe method so GET responses use the same trusted-origin
	// and same-origin rules, including Sec-Fetch-Site behind reverse proxies.
	return p.allowedAsUnsafe(r)
}

// allowedAsUnsafe applies the unsafe-method rules whatever the request's method.
// Use it for GETs that can change something, which browsers send from any site.
func (p *crossOriginProtection) allowedAsUnsafe(r *http.Request) bool {
	check := new(http.Request)
	*check = *r
	check.Method = http.MethodPost
	return p.protection.Check(check) == nil
}

func (p *crossOriginProtection) corsMiddleware(next http.Handler) http.Handler {
	handler := cors.New(cors.Options{
		AllowOriginFunc: p.allowCORS,
		AllowedMethods:  []string{http.MethodHead, http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		AllowedHeaders:  []string{"*"},
	}).Handler(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Origin alone is insufficient: the decision also depends on the key
		// and the browser's same-origin signal. Query keys are part of the URL.
		w.Header().Add("Vary", session.ApiKeyHeader)
		w.Header().Add("Vary", "Sec-Fetch-Site")
		handler.ServeHTTP(w, r)
	})
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
	return p.allowedAsUnsafe(r)
}

func (p *crossOriginProtection) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(authenticatedAPIKeyCtxKey) != true {
			allowed := p.protection.Check(r) == nil
			// GraphQL queries can have side effects, such as running a scraper,
			// so a GET to the API is treated the same as a POST.
			if websocket.IsWebSocketUpgrade(r) || r.URL.Path == gqlEndpoint {
				allowed = p.allowedAsUnsafe(r)
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
