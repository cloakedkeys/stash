package scene

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type oshashNaming struct{}

func (oshashNaming) GetVideoFileNamingAlgorithm() models.HashAlgorithm {
	return models.HashAlgorithmOshash
}

// #2038: merging scenes left the markers' video previews behind.
func TestMergeMovesMarkerFiles(t *testing.T) {
	const seconds = 30
	src := &models.Scene{ID: 1, OSHash: "1111111111111111"}
	dest := &models.Scene{ID: 2, OSHash: "2222222222222222"}

	p := paths.NewPaths(t.TempDir(), "")
	files := func(hash string) []string {
		return []string{
			p.SceneMarkers.GetVideoPreviewPath(hash, seconds),
			p.SceneMarkers.GetWebpPreviewPath(hash, seconds),
			p.SceneMarkers.GetScreenshotPath(hash, seconds),
		}
	}
	for _, f := range files(src.OSHash) {
		require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o755))
		require.NoError(t, os.WriteFile(f, []byte(f), 0o644))
	}

	db := mocks.NewDatabase()
	marker := &models.SceneMarker{ID: 10, SceneID: src.ID, Seconds: seconds}
	db.SceneMarker.On("FindBySceneID", mock.Anything, src.ID).Return([]*models.SceneMarker{marker}, nil)
	db.SceneMarker.On("Update", mock.Anything, marker).Return(nil)

	svc := &Service{MarkerRepository: db.SceneMarker, Paths: &p, Config: oshashNaming{}}
	require.NoError(t, txn.WithTxn(context.Background(), db, func(ctx context.Context) error {
		return svc.mergeSceneMarkers(ctx, dest, src)
	}))

	require.Equal(t, dest.ID, marker.SceneID)
	for i, f := range files(dest.OSHash) {
		require.FileExists(t, f, "moved to the kept scene")
		require.NoFileExists(t, files(src.OSHash)[i], "not left behind")
	}
}
