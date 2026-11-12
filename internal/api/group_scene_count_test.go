//go:build integration

package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stashapp/stash/internal/api/loaders"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGroupSceneCountBatchesDirectCounts(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	db.Scene.On("GetManyGroupSceneCount", mock.Anything, mock.MatchedBy(func(ids []int) bool {
		return len(ids) == 2 && ((ids[0] == 10 && ids[1] == 20) || (ids[0] == 20 && ids[1] == 10))
	})).Return([]int{5, 5}, nil).Once()
	handler := (loaders.Middleware{Repository: r.repository}).Middleware(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		var wg sync.WaitGroup
		counts := make([]int, 3)
		errs := make([]error, 3)
		for i, id := range []int{10, 20, 10} {
			wg.Add(1)
			go func(i, id int) {
				defer wg.Done()
				counts[i], errs[i] = r.Group().SceneCount(req.Context(), &models.Group{ID: id}, nil)
			}(i, id)
		}
		wg.Wait()
		for i := range counts {
			require.NoError(t, errs[i])
			require.Equal(t, 5, counts[i])
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	db.Scene.AssertExpectations(t)
}
