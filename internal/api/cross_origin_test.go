package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	gqlTransport "github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stretchr/testify/require"
)

const originTestMutation = `mutation { voiceActorCreate(input:{name:"Voice"}) { id name } }`

func isolateOriginTestLimiters(t *testing.T) {
	t.Helper()
	oldIP, oldUser := loginFailureLimiter, userLoginFailureLimiter
	loginFailureLimiter = newFailureLimiter(loginFailureLimit, loginFailureWindow)
	userLoginFailureLimiter = newFailureLimiter(userLoginFailureLimit, userLoginFailureWindow)
	t.Cleanup(func() {
		loginFailureLimiter, userLoginFailureLimiter = oldIP, oldUser
	})
}

type originTestUsers struct {
	*apiTestUsers
	single bool
}

func (u *originTestUsers) IsSingleUserMode() bool { return u.single }
func (u *originTestUsers) GetSingleUser(context.Context) (*models.User, error) {
	return u.keys["admin-key"], nil
}

type originTestConfig struct {
	*config.Config
	setup bool
}

func (c originTestConfig) IsNewSystem() bool { return c.setup }
func (c originTestConfig) GetNewSystemCredentials() (string, string, time.Time) {
	return "", "", time.Time{}
}

// Use production authentication, CORS, origin protection, and gqlgen transports
// in the same order as Initialize, without starting the full library manager.
func originTestRouter(t *testing.T, single, setup bool) (http.Handler, *apiVoiceActorStore, *http.Cookie) {
	t.Helper()
	cfg := originTestConfig{Config: config.InitializeEmpty(), setup: setup}
	cfg.SetString(config.SessionStoreKey, "origin-test-cookie-secret")
	cfg.SetInt(config.MaxSessionAge, 3600)
	cfg.SetInterface(config.TrustedOrigins, []string{"https://trusted.example", "http://localhost:3000"})
	users := &originTestUsers{single: single, apiTestUsers: &apiTestUsers{keys: map[string]*models.User{
		"admin-key": {ID: 1, Username: "reader", Roles: models.Roles{models.RoleEnumAdmin}},
	}}}
	db := mocks.NewDatabase()
	tx := db.Repository().TxnManager
	sessions := session.NewStore(cfg, users, &tx)
	resolver := newResolver(db)
	store := &apiVoiceActorStore{}
	resolver.repository.VoiceActor = store
	gql := newTestGraphQLServer(resolver)
	p, err := newCrossOriginProtection(cfg.GetTrustedOrigins())
	require.NoError(t, err)
	gql.AddTransport(gqlTransport.Websocket{Upgrader: websocket.Upgrader{CheckOrigin: p.checkWebsocketOrigin}})
	gql.AddTransport(gqlTransport.Options{})
	gql.AddTransport(gqlTransport.GET{})
	gql.AddTransport(gqlTransport.MultipartForm{MaxUploadSize: 1024 * 1024})
	r := chi.NewRouter()
	r.Use(p.corsMiddleware)
	r.Use(RequestIPMiddleware)
	r.Use(authenticateHandlerWithServices(tx, cfg, func() UserAuthenticator { return users }, sessions))
	r.Use(p.corsMiddleware)
	r.Use(p.middleware)
	r.Handle(gqlEndpoint, gql)
	r.HandleFunc("/scene/*", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Post(loginEndpoint, handleLoginPost(sessions, cfg))
	login := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=reader&password=reader-password"))
	login.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	require.NoError(t, sessions.Login(w, login))
	return r, store, w.Result().Cookies()[0]
}

func TestCrossOriginHTTP(t *testing.T) {
	isolateOriginTestLimiters(t)
	for _, single := range []bool{true, false} {
		mode := "password"
		if single {
			mode = "no-password"
		}
		t.Run(mode, func(t *testing.T) {
			wrongKeyStatus, wrongKeyNoOriginStatus := http.StatusUnauthorized, http.StatusUnauthorized
			if single {
				wrongKeyStatus, wrongKeyNoOriginStatus = http.StatusForbidden, http.StatusOK
			}
			for _, tc := range []struct {
				name, origin, fetchSite, key string
				form, queryKey               bool
				want                         int
			}{
				{name: "cross-site form", origin: "https://evil.example", fetchSite: "cross-site", form: true, want: 403},
				{name: "cross-site JSON", origin: "https://evil.example", fetchSite: "cross-site", want: 403},
				{name: "origin fallback refuses", origin: "https://evil.example", want: 403},
				{name: "same-origin", origin: "http://stash.test", fetchSite: "same-origin", want: 200},
				{name: "same-origin form", origin: "http://stash.test", fetchSite: "same-origin", form: true, want: 200},
				{name: "same-origin fallback", origin: "http://stash.test", want: 200},
				{name: "proxy changed Host", origin: "https://public.example", fetchSite: "same-origin", want: 200},
				{name: "curl", want: 200},
				{name: "valid key", origin: "https://evil.example", fetchSite: "cross-site", key: "admin-key", want: 200},
				{name: "valid query key", origin: "https://evil.example", fetchSite: "cross-site", key: "admin-key", queryKey: true, want: 200},
				{name: "wrong key", origin: "https://evil.example", fetchSite: "cross-site", key: "wrong-key", want: wrongKeyStatus},
				{name: "wrong key without origin", key: "wrong-key", want: wrongKeyNoOriginStatus},
				{name: "trusted", origin: "https://trusted.example", fetchSite: "cross-site", want: 200},
				{name: "trusted form", origin: "https://trusted.example", fetchSite: "cross-site", form: true, want: 200},
				{name: "dev server", origin: "http://localhost:3000", fetchSite: "same-site", want: 200},
				{name: "untrusted port", origin: "http://stash.test:3001", fetchSite: "same-site", want: 403},
			} {
				t.Run(tc.name, func(t *testing.T) {
					isolateOriginTestLimiters(t)
					router, store, cookie := originTestRouter(t, single, false)
					payload, err := json.Marshal(map[string]string{"query": originTestMutation})
					require.NoError(t, err)
					body := bytes.NewBuffer(payload)
					contentType := "application/json"
					if tc.form {
						body.Reset()
						form := multipart.NewWriter(body)
						require.NoError(t, form.WriteField("operations", string(payload)))
						require.NoError(t, form.WriteField("map", `{}`))
						require.NoError(t, form.Close())
						contentType = form.FormDataContentType()
					}
					req := httptest.NewRequest(http.MethodPost, "http://stash.test/graphql", body)
					req.RemoteAddr = "127.0.0.1:12345"
					req.Header.Set("Content-Type", contentType)
					req.Header.Set("Origin", tc.origin)
					req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
					if tc.queryKey {
						req.URL.RawQuery = "apikey=" + tc.key
					} else {
						req.Header.Set(session.ApiKeyHeader, tc.key)
					}
					if !single && tc.key == "" {
						// A valid session must not exempt cross-site requests.
						req.AddCookie(cookie)
					}
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)
					require.Equal(t, tc.want, w.Code, w.Body.String())
					if single && tc.key == "wrong-key" {
						require.Empty(t, loginFailureLimiter.entries, "ignored keys must not count as login failures")
					}
					if tc.want == http.StatusOK {
						require.NotContains(t, w.Body.String(), `"errors"`)
						require.NotNil(t, store.actor, w.Body.String())
					} else {
						require.Nil(t, store.actor)
						if tc.want == http.StatusForbidden {
							require.Contains(t, w.Body.String(), "trusted_origins")
							require.Contains(t, w.Header().Get("Content-Type"), "text/plain")
						}
					}
				})
			}
		})
	}
}

func TestCrossOriginWebsocket(t *testing.T) {
	isolateOriginTestLimiters(t)
	for _, tc := range []struct {
		name, origin, fetchSite, key string
		proxyHost                    bool
		want                         int
	}{
		{name: "cross-site", origin: "https://evil.example", fetchSite: "cross-site", want: 403},
		{name: "origin fallback", origin: "https://evil.example", want: 403},
		{name: "same-origin", origin: "self", fetchSite: "same-origin", want: 101},
		{name: "same-origin fallback", origin: "self", want: 101},
		{name: "proxy changed Host", origin: "self", fetchSite: "same-origin", proxyHost: true, want: 101},
		{name: "no origin", want: 101},
		{name: "trusted", origin: "https://trusted.example", fetchSite: "cross-site", want: 101},
		{name: "valid key", origin: "https://evil.example", fetchSite: "cross-site", key: "admin-key", want: 101},
		{name: "wrong key", origin: "https://evil.example", fetchSite: "cross-site", key: "bad-ws-key", want: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, _, _ := originTestRouter(t, true, false)
			srv := httptest.NewServer(router)
			defer srv.Close()
			origin := tc.origin
			if origin == "self" {
				origin = srv.URL
			}
			headers := http.Header{}
			if origin != "" {
				headers.Set("Origin", origin)
			}
			headers.Set("Sec-Fetch-Site", tc.fetchSite)
			headers.Set(session.ApiKeyHeader, tc.key)
			if tc.proxyHost {
				headers.Set("Host", "backend.internal:9999")
			}
			dialer := websocket.Dialer{Subprotocols: []string{"graphql-transport-ws"}, HandshakeTimeout: 5 * time.Second}
			conn, response, err := dialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+gqlEndpoint, headers)
			require.NotNil(t, response)
			defer response.Body.Close()
			if conn != nil {
				defer conn.Close()
			}
			require.Equal(t, tc.want, response.StatusCode)
			if tc.want != http.StatusSwitchingProtocols {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
			require.NoError(t, conn.WriteJSON(map[string]string{"type": "connection_init"}))
			var ack map[string]interface{}
			require.NoError(t, conn.ReadJSON(&ack))
			require.Equal(t, "connection_ack", ack["type"])
			require.NoError(t, conn.WriteJSON(map[string]interface{}{
				"id": "1", "type": "subscribe", "payload": map[string]string{"query": originTestMutation},
			}))
			var result map[string]interface{}
			require.NoError(t, conn.ReadJSON(&result))
			require.Equal(t, "next", result["type"])
			payload, err := json.Marshal(result["payload"])
			require.NoError(t, err)
			require.Contains(t, string(payload), `"id":"42"`)
			require.NotContains(t, string(payload), `"errors"`)
		})
	}
}

func TestCrossOriginSetupLoginAndGET(t *testing.T) {
	isolateOriginTestLimiters(t)
	for _, setup := range []bool{false, true} {
		for _, tc := range []struct {
			name, method, path, origin, fetchSite string
			want                                  int
		}{
			{"login same-origin", "POST", "/login", "http://stash.test", "same-origin", 200},
			{"login cross-site", "POST", "/login", "https://evil.example", "cross-site", 403},
			{"API same-origin", "POST", "/graphql", "http://stash.test", "same-origin", 200},
			{"API cross-site", "POST", "/graphql", "https://evil.example", "cross-site", 403},
			{"preflight", "OPTIONS", "/graphql", "https://evil.example", "cross-site", 200},
			{"query GET", "GET", "/graphql?query=" + url.QueryEscape(`{ __typename }`), "https://evil.example", "cross-site", 200},
			{"mutation GET", "GET", "/graphql?query=" + url.QueryEscape(originTestMutation), "https://evil.example", "cross-site", 406},
		} {
			t.Run(tc.name+"/setup="+strconv.FormatBool(setup), func(t *testing.T) {
				router, store, _ := originTestRouter(t, true, setup)
				req := httptest.NewRequest(tc.method, "http://stash.test"+tc.path, strings.NewReader("username=reader&password=reader-password"))
				if setup {
					req = httptest.NewRequest(tc.method, "http://stash.test"+tc.path, strings.NewReader("username=&password="))
				}
				req.RemoteAddr = "127.0.0.1:12345"
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if tc.method == http.MethodPost && tc.path == gqlEndpoint {
					req.Body = io.NopCloser(strings.NewReader(`{"query":"{ __typename }"}`))
					req.Header.Set("Content-Type", "application/json")
				}
				req.Header.Set("Origin", tc.origin)
				req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
				if tc.method == http.MethodOptions {
					req.Header.Set("Access-Control-Request-Method", "POST")
					req.Header.Set("Access-Control-Request-Headers", "Content-Type,ApiKey")
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.Equal(t, tc.want, w.Code, w.Body.String())
				require.Nil(t, store.actor)
			})
		}
	}
}

func TestCrossOriginInvalidTrustedOrigins(t *testing.T) {
	for _, origin := range []string{"example.com", "https://example.com/", "https://example.com/path", "https://example.com?query", "https://example.com#fragment"} {
		t.Run(origin, func(t *testing.T) {
			_, err := newCrossOriginProtection([]string{origin})
			require.ErrorContains(t, err, "trusted_origins")
		})
	}
}

func TestCrossOriginResponseAccess(t *testing.T) {
	isolateOriginTestLimiters(t)
	for _, single := range []bool{true, false} {
		for _, tc := range []struct {
			name, origin, fetchSite, key, preflightHeaders string
			post, queryKey                                 bool
			wantOrigin                                     string
		}{
			{name: "untrusted", origin: "https://evil.example", fetchSite: "cross-site"},
			{name: "null origin", origin: "null", fetchSite: "cross-site"},
			{name: "trusted", origin: "https://trusted.example", fetchSite: "cross-site", wantOrigin: "https://trusted.example"},
			{name: "development UI", origin: "http://localhost:3000", fetchSite: "same-site", post: true, wantOrigin: "http://localhost:3000"},
			{name: "valid key", origin: "https://evil.example", fetchSite: "cross-site", key: "admin-key", wantOrigin: "https://evil.example"},
			{name: "extension key", origin: "chrome-extension://test-extension", key: "admin-key", wantOrigin: "chrome-extension://test-extension"},
			{name: "valid query key", origin: "https://evil.example", fetchSite: "cross-site", key: "admin-key", queryKey: true, wantOrigin: "https://evil.example"},
			{name: "wrong key", origin: "https://evil.example", fetchSite: "cross-site", key: "wrong-key"},
			{name: "key preflight", origin: "https://evil.example", preflightHeaders: "content-type, aPiKeY", wantOrigin: "https://evil.example"},
			{name: "untrusted preflight", origin: "https://evil.example", preflightHeaders: "Content-Type"},
			{name: "similar header preflight", origin: "https://evil.example", preflightHeaders: "X-ApiKey"},
			{name: "trusted preflight", origin: "https://trusted.example", preflightHeaders: "Content-Type", wantOrigin: "https://trusted.example"},
			{name: "same origin", origin: "http://stash.test", fetchSite: "same-origin", post: true, wantOrigin: "http://stash.test"},
			{name: "same origin fallback", origin: "http://stash.test", post: true, wantOrigin: "http://stash.test"},
			{name: "proxy changed Host", origin: "https://public.example", fetchSite: "same-origin", post: true, wantOrigin: "https://public.example"},
			{name: "no origin"},
		} {
			t.Run(tc.name+"/single="+strconv.FormatBool(single), func(t *testing.T) {
				router, _, cookie := originTestRouter(t, single, false)
				req := httptest.NewRequest(http.MethodGet, "http://stash.test/graphql?query="+url.QueryEscape(`{ __typename }`), nil)
				req.RemoteAddr = "127.0.0.1:12345"
				if tc.post {
					req.Method = http.MethodPost
					req.Body = io.NopCloser(strings.NewReader(`{"query":"{ __typename }"}`))
					req.Header.Set("Content-Type", "application/json")
				}
				if tc.preflightHeaders != "" {
					req.Method = http.MethodOptions
					req.Header.Set("Access-Control-Request-Method", "POST")
					req.Header.Set("Access-Control-Request-Headers", tc.preflightHeaders)
				} else if !single && tc.key == "" {
					req.AddCookie(cookie)
				}
				req.Header.Set("Origin", tc.origin)
				req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
				if tc.queryKey {
					req.URL.RawQuery += "&apikey=" + tc.key
				} else {
					req.Header.Set(session.ApiKeyHeader, tc.key)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				wantStatus := http.StatusOK
				if !single && tc.key == "wrong-key" {
					wantStatus = http.StatusUnauthorized
				}
				require.Equal(t, wantStatus, w.Code, w.Body.String())
				require.Equal(t, tc.wantOrigin, w.Header().Get("Access-Control-Allow-Origin"))
				if tc.wantOrigin != "" {
					require.Equal(t, []string{tc.wantOrigin}, w.Header().Values("Access-Control-Allow-Origin"))
				}
				if tc.wantOrigin != "" {
					vary := strings.Join(w.Header().Values("Vary"), ", ")
					require.Contains(t, vary, "Origin")
					require.Contains(t, vary, session.ApiKeyHeader)
					require.Contains(t, vary, "Sec-Fetch-Site")
				}
				if tc.preflightHeaders != "" && tc.wantOrigin != "" {
					vary := strings.Join(w.Header().Values("Vary"), ", ")
					require.Contains(t, vary, "Access-Control-Request-Method")
					require.Contains(t, vary, "Access-Control-Request-Headers")
					require.Equal(t, "POST", w.Header().Get("Access-Control-Allow-Methods"))
					require.Equal(t, strings.ToLower(tc.preflightHeaders), strings.ToLower(w.Header().Get("Access-Control-Allow-Headers")))
				} else if wantStatus == http.StatusOK && tc.preflightHeaders == "" {
					require.Contains(t, w.Body.String(), `"__typename":"Query"`)
				}
			})
		}
	}
}

func TestCrossOriginCastResponseAccess(t *testing.T) {
	for _, tc := range []struct {
		name, origin, path, method string
		allowed                    bool
	}{
		{"HTTPS stream", "https://www.gstatic.com", "/scene/1/stream.m3u8", "GET", true},
		{"HTTP stream", "http://www.gstatic.com", "/scene/1/stream.m3u8", "GET", true},
		{"segment", "https://www.gstatic.com", "/scene/1/stream/segment.ts", "GET", true},
		{"captions", "https://www.gstatic.com", "/scene/1/caption.vtt", "GET", true},
		{"HEAD", "https://www.gstatic.com", "/scene/1/stream.m3u8", "HEAD", true},
		{"preflight", "https://www.gstatic.com", "/scene/1/stream.m3u8", "OPTIONS", true},
		{"HTTPS GraphQL", "https://www.gstatic.com", "/graphql", "GET", false},
		{"HTTP GraphQL", "http://www.gstatic.com", "/graphql", "GET", false},
		{"similar host", "https://www.gstatic.com.evil.example", "/scene/1/stream.m3u8", "GET", false},
		{"other origin", "https://evil.example", "/scene/1/stream.m3u8", "GET", false},
		{"similar path", "https://www.gstatic.com", "/scene-other/1/stream.m3u8", "GET", false},
		{"write", "https://www.gstatic.com", "/scene/1/stream.m3u8", "POST", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, _, _ := originTestRouter(t, true, false)
			req := httptest.NewRequest(tc.method, "http://stash.test"+tc.path, nil)
			req.RemoteAddr = "127.0.0.1:12345"
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			if tc.method == http.MethodOptions {
				req.Header.Set("Access-Control-Request-Method", "GET")
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if tc.allowed {
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, []string{tc.origin}, w.Header().Values("Access-Control-Allow-Origin"))
			} else {
				require.Empty(t, w.Header().Values("Access-Control-Allow-Origin"))
			}
		})
	}
}

func TestCrossOriginUnauthorizedResponseAccess(t *testing.T) {
	for _, origin := range []string{"https://trusted.example", "https://evil.example"} {
		for _, key := range []string{"", "wrong-key"} {
			t.Run(origin+"/key="+key, func(t *testing.T) {
				isolateOriginTestLimiters(t)
				router, _, _ := originTestRouter(t, false, false)
				req := httptest.NewRequest(http.MethodGet, "http://stash.test/graphql", nil)
				req.RemoteAddr = "127.0.0.1:12345"
				req.Header.Set("Origin", origin)
				req.Header.Set("Sec-Fetch-Site", "cross-site")
				req.Header.Set(session.ApiKeyHeader, key)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.Equal(t, http.StatusUnauthorized, w.Code)
				if origin == "https://trusted.example" {
					require.Equal(t, []string{origin}, w.Header().Values("Access-Control-Allow-Origin"))
				} else {
					require.Empty(t, w.Header().Values("Access-Control-Allow-Origin"))
				}
				vary := strings.Join(w.Header().Values("Vary"), ", ")
				require.Contains(t, vary, "Origin")
				require.Contains(t, vary, session.ApiKeyHeader)
				require.Contains(t, vary, "Sec-Fetch-Site")
			})
		}
	}
}
