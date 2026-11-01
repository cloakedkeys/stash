package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"

	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/api/loaders"
	"github.com/stashapp/stash/internal/api/urlbuilders"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/plugin/hook"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func performerImageQuery(t *testing.T, h http.Handler, query string, role models.RoleEnum) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": query})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(session.SetCurrentUser(req.Context(), models.User{ID: 1, Roles: models.Roles{role}}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	return w.Body.String()
}

type performerImageHooks struct {
	calls  int
	fields []string
}

func (h *performerImageHooks) ExecutePostHooks(_ context.Context, _ int, trigger hook.TriggerEnum, _ interface{}, fields []string) {
	if trigger == hook.PerformerUpdatePost {
		h.calls++
		h.fields = fields
	}
}

func TestPerformerImageMutations(t *testing.T) {
	for _, name := range []string{"performerImageAdd", "performerImageRemove", "performerImageSetPrimary", "performerImagesReorder"} {
		t.Run(name, func(t *testing.T) {
			db := mocks.NewDatabase()
			r := newResolver(db)
			hooks := &performerImageHooks{}
			r.hookExecutor = hooks
			srv := newTestGraphQLServer(r)
			args := `id:"1", checksum:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`
			method := "RemoveAlternateImage"
			var value interface{} = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			switch name {
			case "performerImageAdd":
				args = `id:"1", image:"data:image/png;base64,cG9ydHJhaXQ="`
				method = "AddAlternateImage"
				value = []byte("portrait")
			case "performerImageSetPrimary":
				method = "SetPrimaryImage"
			case "performerImagesReorder":
				args = `id:"1", checksums:["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]`
				method = "ReorderAlternateImages"
				value = []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
			}
			query := "mutation { " + name + "(" + args + ") { id } }"
			// A read account must be refused before image processing or storage.
			require.Contains(t, performerImageQuery(t, srv, query, models.RoleEnumRead), "unauthorized")
			db.Performer.On("Find", mock.Anything, 1).Return(&models.Performer{ID: 1}, nil)
			call := db.Performer.On(method, mock.Anything, 1, value).Once()
			if name == "performerImageAdd" {
				call.Return(true, nil)
			} else {
				call.Return(nil)
			}
			db.Performer.On("UpdatePartial", mock.Anything, 1, mock.Anything).Return(&models.Performer{ID: 1}, nil).Once()
			require.NotContains(t, performerImageQuery(t, srv, query, models.RoleEnumModify), "errors")
			require.Equal(t, 1, hooks.calls)
			require.Contains(t, hooks.fields, "alternate_images")
			if name == "performerImageSetPrimary" {
				require.Contains(t, hooks.fields, "image")
			}
			db.AssertExpectations(t)
		})
	}
	for _, method := range []string{"RemoveAlternateImage", "SetPrimaryImage", "ReorderAlternateImages"} {
		t.Run(method+" foreign checksum", func(t *testing.T) {
			db := mocks.NewDatabase()
			r := newResolver(db)
			db.Performer.On("Find", mock.Anything, 2).Return(&models.Performer{ID: 2}, nil)
			switch method {
			case "RemoveAlternateImage":
				db.Performer.On(method, mock.Anything, 2, "foreign").Return(sql.ErrNoRows).Once()
				_, err := r.Mutation().PerformerImageRemove(context.Background(), "2", "foreign")
				require.ErrorContains(t, err, "does not belong to performer 2")
			case "SetPrimaryImage":
				db.Performer.On(method, mock.Anything, 2, "foreign").Return(sql.ErrNoRows).Once()
				_, err := r.Mutation().PerformerImageSetPrimary(context.Background(), "2", "foreign")
				require.ErrorContains(t, err, "does not belong to performer 2")
			case "ReorderAlternateImages":
				db.Performer.On(method, mock.Anything, 2, []string{"foreign"}).Return(sql.ErrNoRows).Once()
				_, err := r.Mutation().PerformerImagesReorder(context.Background(), "2", []string{"foreign"})
				require.ErrorContains(t, err, "does not belong to performer 2")
			}
			db.AssertExpectations(t)
		})
	}
}

func TestPerformerImageAddFormsAndProtection(t *testing.T) {
	data := []byte("portrait")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write(data)
		require.NoError(t, err)
	}))
	defer server.Close()
	for _, input := range []string{"data:image/png;base64,cG9ydHJhaXQ=", server.URL} {
		db := mocks.NewDatabase()
		r := newResolver(db)
		db.Performer.On("Find", mock.Anything, 1).Return(&models.Performer{ID: 1}, nil)
		db.Performer.On("AddAlternateImage", mock.Anything, 1, data).Return(true, nil).Once()
		db.Performer.On("UpdatePartial", mock.Anything, 1, mock.Anything).Return(&models.Performer{ID: 1}, nil).Once()
		_, err := r.Mutation().PerformerImageAdd(context.Background(), "1", input)
		require.NoError(t, err)
		db.AssertExpectations(t)
	}
	data = []byte("<html><script>alert(1)</script></html>")
	db := mocks.NewDatabase()
	r := newResolver(db)
	for _, input := range []string{"data:text/html;base64,PGh0bWw+", server.URL, "", "data:image/png;base64,!!!!"} {
		_, err := r.Mutation().PerformerImageAdd(context.Background(), "1", input)
		require.Error(t, err)
	}
	// No storage call is expected for invalid data.
	db.AssertExpectations(t)
}

func TestPerformerListAlternateImagesAreLazyAndBatched(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	people := []*models.Performer{{ID: 1}, {ID: 2}}
	db.Performer.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(people, 2, nil)
	srv := newTestGraphQLServer(r)
	h := loaders.Middleware{Repository: r.repository}.Middleware(srv)
	require.NotContains(t, performerImageQuery(t, h, "{ findPerformers { performers { id } } }", models.RoleEnumRead), "errors")
	db.Performer.AssertNotCalled(t, "GetAlternateImagesBulk", mock.Anything, mock.Anything)
	db.Performer.On("GetAlternateImagesBulk", mock.Anything, mock.MatchedBy(func(ids []int) bool { return len(ids) == 2 && ids[0] != ids[1] })).Return([][]models.PerformerImage{{{Checksum: "a"}}, {{Checksum: "b"}}}, nil).Once()
	result := performerImageQuery(t, h, "{ findPerformers { performers { id alternate_images { checksum path } } } }", models.RoleEnumRead)
	require.NotContains(t, result, "errors")
	require.Contains(t, result, "/image/a?t=a")
	// GetImage/GetAlternateImage have no expectations and would fail if bytes were read.
	db.AssertExpectations(t)
}

func TestPerformerAlternateImageRoute(t *testing.T) {
	db := mocks.NewDatabase()
	checksum := strings.Repeat("a", 32)
	db.Performer.On("Find", mock.Anything, 1).Return(&models.Performer{ID: 1}, nil)
	db.Performer.On("Find", mock.Anything, 2).Return(&models.Performer{ID: 2}, nil)
	db.Performer.On("Find", mock.Anything, 3).Return(nil, nil)
	db.Performer.On("GetAlternateImage", mock.Anything, 1, checksum).Return([]byte("portrait"), nil)
	db.Performer.On("GetAlternateImage", mock.Anything, 2, checksum).Return(nil, sql.ErrNoRows)
	rs := performerRoutes{routes: routes{txnManager: db.Repository().TxnManager}, performerFinder: db.Performer}
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/1/image/" + checksum, 200}, {"/2/image/" + checksum, 404}, {"/3/image/" + checksum, 404},
		{"/1/image/invalid", 400}, {"/1/image/" + strings.Repeat("a", 31), 400}, {"/1/image/" + strings.Repeat("g", 32), 400},
	} {
		w := httptest.NewRecorder()
		rs.Routes().ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		require.Equal(t, tc.status, w.Code, tc.path)
		if tc.status == 200 {
			require.Equal(t, "portrait", w.Body.String())
			require.Equal(t, "private, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
			require.Contains(t, w.Header().Get("Content-Security-Policy"), "sandbox")
		}
	}
	// The same Find call as the primary route enforces the store's library limit.
	db.Performer.ExpectedCalls = nil
	db.Performer.On("Find", mock.MatchedBy(func(ctx context.Context) bool { return models.LibraryLimitFromContext(ctx) != nil }), 1).Return(nil, nil).Once()
	req := httptest.NewRequest("GET", "/1/image/"+checksum, nil)
	req = req.WithContext(models.WithLibraryLimit(req.Context(), &models.LibraryLimit{TagIDs: []int{99}}))
	w := httptest.NewRecorder()
	rs.Routes().ServeHTTP(w, req)
	require.Equal(t, 404, w.Code)
	db.AssertExpectations(t)
}

func TestAlternateImageURLOmitsAPIKey(t *testing.T) {
	config.InitializeEmpty()
	h := BaseURLMiddleware(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		base, _ := req.Context().Value(BaseURLCtxKey).(string)
		_, err := w.Write([]byte(urlbuilders.NewPerformerURLBuilder(base, &models.Performer{ID: 7}).GetAlternateImageURL("abc")))
		require.NoError(t, err)
	}))
	for _, header := range []bool{true, false} {
		req := httptest.NewRequest("GET", "http://example.com/graphql", nil)
		if header {
			req.Header.Set(session.ApiKeyHeader, "a+b")
		} else {
			req.URL.RawQuery = "apikey=a%2Bb"
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		require.Equal(t, "http://example.com/performer/7/image/abc?t=abc", w.Body.String())
	}
}

func TestPerformerAlternateRouteAuthentication(t *testing.T) {
	cfg := config.InitializeEmpty()
	cfg.SetPublicAccess(true)
	cfg.SetString(config.SessionStoreKey, "performer-route-test")
	cfg.SetInt(config.MaxSessionAge, 3600)
	db := mocks.NewDatabase()
	tx := db.Repository().TxnManager
	users := &apiTestUsers{keys: map[string]*models.User{"read-key": {ID: 1, Username: "reader", Roles: models.Roles{models.RoleEnumRead}}}}
	sessions := session.NewStore(cfg, users, &tx)
	rs := performerRoutes{routes: routes{txnManager: tx}, performerFinder: db.Performer}
	h := authenticateHandlerWithServices(tx, cfg, func() UserAuthenticator { return users }, sessions)(rs.Routes())
	checksum := strings.Repeat("a", 32)
	db.Performer.On("Find", mock.Anything, 1).Return(&models.Performer{ID: 1}, nil).Once()
	db.Performer.On("GetAlternateImage", mock.Anything, 1, checksum).Return([]byte("portrait"), nil).Once()
	for _, tc := range []struct {
		key    string
		status int
	}{{"", 302}, {"invalid-key", 401}, {"read-key", 200}} {
		req := httptest.NewRequest("GET", "/1/image/"+checksum, nil)
		req.Header.Set(session.ApiKeyHeader, tc.key)
		req = req.WithContext(context.WithValue(req.Context(), IPCtxKey, net.ParseIP("127.0.0.1")))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code)
	}
	db.AssertExpectations(t)
}

func TestPerformerImagesRefreshBetweenMutationFields(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	db.Performer.On("Find", mock.Anything, 1).Return(&models.Performer{ID: 1}, nil)
	db.Performer.On("AddAlternateImage", mock.Anything, 1, []byte("a")).Return(true, nil).Once()
	db.Performer.On("AddAlternateImage", mock.Anything, 1, []byte("b")).Return(true, nil).Once()
	db.Performer.On("UpdatePartial", mock.Anything, 1, mock.Anything).Return(&models.Performer{ID: 1}, nil).Twice()
	db.Performer.On("GetAlternateImagesBulk", mock.Anything, []int{1}).Return([][]models.PerformerImage{{{Checksum: "a"}}}, nil).Once()
	db.Performer.On("GetAlternateImagesBulk", mock.Anything, []int{1}).Return([][]models.PerformerImage{{{Checksum: "a"}, {Checksum: "b"}}}, nil).Once()
	h := loaders.Middleware{Repository: r.repository}.Middleware(newTestGraphQLServer(r))
	result := performerImageQuery(t, h, `mutation {
 first: performerImageAdd(id:"1",image:"data:image/png;base64,YQ==") { alternate_images { checksum } }
 second: performerImageAdd(id:"1",image:"data:image/png;base64,Yg==") { alternate_images { checksum } }
 }`, models.RoleEnumModify)
	require.NotContains(t, result, "errors")
	require.Contains(t, result, `"second":{"alternate_images":[{"checksum":"a"},{"checksum":"b"}]}`)
	db.AssertExpectations(t)
}
