package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedCaptionRouteAccess(t *testing.T) {
	cfg := config.InitializeEmpty()
	cfg.SetPublicAccess(true)
	cfg.SetString(config.SessionStoreKey, "embedded-caption-test-secret")
	db := mocks.NewDatabase()
	tx := db.Repository().TxnManager
	limit := &models.LibraryLimit{TagIDs: []int{7}}
	users := &apiTestUsers{keys: map[string]*models.User{
		"read-key": {ID: 1, Username: "reader", Roles: models.Roles{models.RoleEnumRead}, LibraryLimit: limit},
	}}
	sessions := session.NewStore(cfg, users, &tx)
	rs := sceneRoutes{routes: routes{txnManager: tx}, sceneFinder: db.Scene, fileGetter: db.File}
	router := chi.NewRouter()
	router.Mount("/scene", rs.Routes())
	handler := authenticateHandlerWithServices(tx, cfg, func() UserAuthenticator { return users }, sessions)(router)
	limitedCtx := mock.MatchedBy(func(ctx context.Context) bool { return models.LibraryLimitFromContext(ctx) == limit })
	db.Scene.On("Find", limitedCtx, 2).Return(nil, nil)
	db.Scene.On("Find", limitedCtx, 1).Return(&models.Scene{ID: 1}, nil)
	request := func(url, key string) int {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		if key != "" {
			req.Header.Set(session.ApiKeyHeader, key)
		}
		req = req.WithContext(context.WithValue(req.Context(), IPCtxKey, net.ParseIP("127.0.0.1")))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}
	require.Equal(t, http.StatusFound, request("/scene/1/caption?stream=2", ""))
	require.Equal(t, http.StatusUnauthorized, request("/scene/1/caption?stream=2", "bad-key"))
	// Even invalid indices must pass through the same visibility gate as captions.
	require.Equal(t, http.StatusNotFound, request("/scene/2/caption?stream=not-an-index", "read-key"))
	require.Equal(t, http.StatusBadRequest, request("/scene/1/caption?stream=not-an-index", "read-key"))
	require.Equal(t, http.StatusBadRequest, request("/scene/1/caption?stream=2&stream=3", "read-key"))
	db.Scene.AssertExpectations(t)
}
