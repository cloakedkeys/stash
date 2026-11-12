package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSceneStreamSupportsHead(t *testing.T) {
	router := (sceneRoutes{}).Routes()
	if !router.Match(chi.NewRouteContext(), http.MethodHead, "/1/stream") {
		t.Fatal("direct scene stream does not support a HEAD availability check")
	}
}

func TestHashAllowedRequiresExactVisibleGeneratedKey(t *testing.T) {
	start, end := 10.0, 20.0
	for _, algo := range []models.HashAlgorithm{models.HashAlgorithmOshash, models.HashAlgorithmMd5} {
		for _, limited := range []bool{false, true} {
			t.Run(string(algo)+"/"+map[bool]string{true: "limited", false: "unlimited"}[limited], func(t *testing.T) {
				hash := "0123456789abcdef"
				if algo == models.HashAlgorithmMd5 {
					hash += hash
				}
				visible := &models.Scene{ID: 1, OSHash: hash, Checksum: hash, StartTime: &start, EndTime: &end}
				db := mocks.NewDatabase()
				if algo == models.HashAlgorithmOshash {
					db.Scene.On("FindByOSHash", mock.Anything, hash).Return([]*models.Scene{visible}, nil)
				} else {
					db.Scene.On("FindByOSHash", mock.Anything, hash).Return(nil, nil)
					db.Scene.On("FindByChecksum", mock.Anything, hash).Return([]*models.Scene{visible}, nil)
				}
				rs := sceneRoutes{routes: routes{txnManager: db}, sceneFinder: db.Scene}
				ctx := context.Background()
				if limited {
					ctx = models.WithLibraryLimit(ctx, &models.LibraryLimit{})
				}
				req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
				require.True(t, rs.hashAllowed(req, hash+"_10000-20000"))
				require.False(t, rs.hashAllowed(req, hash+"_10000-30000"), "a visible sibling does not authorise another range")
				require.False(t, rs.hashAllowed(req, hash), "a part does not authorise the whole-file key")
				require.False(t, rs.hashAllowed(req, "../"+hash))
			})
		}
	}
}

func TestPartGeneratedKeyRoutes(t *testing.T) {
	for _, key := range []string{"0123456789abcdef_1234-9876", "0123456789abcdef_1234-end"} {
		for _, suffix := range []string{"_sprite.jpg", "_thumbs.vtt"} {
			t.Run(key+suffix, func(t *testing.T) {
				db := mocks.NewDatabase()
				db.Scene.On("FindByOSHash", mock.Anything, "0123456789abcdef").Return(nil, nil).Once()
				db.Scene.On("FindByChecksum", mock.Anything, "0123456789abcdef").Return(nil, nil).Once()
				router := (sceneRoutes{routes: routes{txnManager: db}, sceneFinder: db.Scene}).Routes()
				// Exercise the mounted router; the scene-ID branch would return 400.
				request := httptest.NewRequest(http.MethodGet, "/"+key+suffix, nil)
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, request)
				require.Equal(t, http.StatusNotFound, recorder.Code)
				db.Scene.AssertExpectations(t)
				routeCtx := chi.NewRouteContext()
				require.True(t, router.Match(routeCtx, http.MethodGet, "/"+key+suffix))
				request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeCtx))
				require.Equal(t, key, generatedKeyParam(request))
			})
		}
	}
}
