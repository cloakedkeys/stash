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

const markerSeconds = 30

func markerFiles(p paths.Paths, hash string) []string {
	return []string{
		p.SceneMarkers.GetVideoPreviewPath(hash, markerSeconds),
		p.SceneMarkers.GetWebpPreviewPath(hash, markerSeconds),
		p.SceneMarkers.GetScreenshotPath(hash, markerSeconds),
	}
}

// mergeMarker moves a marker at markerSeconds, with all of its generated
// files, from src to the destination, whose hash once it has its files is
// destHash.
func mergeMarker(t *testing.T, src *models.Scene, destID int, destHash string) paths.Paths {
	t.Helper()
	p := paths.NewPaths(t.TempDir(), "")
	for _, f := range markerFiles(p, src.OSHash) {
		require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o755))
		require.NoError(t, os.WriteFile(f, []byte(f), 0o644))
	}

	db := mocks.NewDatabase()
	marker := &models.SceneMarker{ID: 10, SceneID: src.ID, Seconds: markerSeconds}
	db.SceneMarker.On("FindBySceneID", mock.Anything, src.ID).Return([]*models.SceneMarker{marker}, nil)
	db.SceneMarker.On("Update", mock.Anything, marker).Return(nil)

	svc := &Service{MarkerRepository: db.SceneMarker, Paths: &p, Config: oshashNaming{}}
	require.NoError(t, txn.WithTxn(context.Background(), db, func(ctx context.Context) error {
		moved, err := svc.mergeSceneMarkers(ctx, destID, src)
		if err != nil {
			return err
		}
		svc.moveMarkerFiles(ctx, moved, destHash)
		return nil
	}))

	require.Equal(t, destID, marker.SceneID)
	return p
}

// #2038: merging scenes left the markers' video previews behind.
func TestMergeMovesMarkerFiles(t *testing.T) {
	src := &models.Scene{ID: 1, OSHash: "1111111111111111"}
	const destHash = "2222222222222222"
	p := mergeMarker(t, src, 2, destHash)

	for i, f := range markerFiles(p, destHash) {
		require.FileExists(t, f, "moved to the kept scene")
		require.NoFileExists(t, markerFiles(p, src.OSHash)[i], "not left behind")
	}
}

// A kept scene that had no file takes the first source's file, and its hash,
// so that source's marker files are already in the right place.
func TestMergeKeepsMarkerFilesOnTheSameHash(t *testing.T) {
	src := &models.Scene{ID: 1, OSHash: "1111111111111111"}
	p := mergeMarker(t, src, 2, src.OSHash)

	for _, f := range markerFiles(p, src.OSHash) {
		require.FileExists(t, f)
	}
}

// If the kept scene still has no file, there's nowhere to move the files to.
func TestMergeIntoSceneWithoutFileKeepsMarkerFiles(t *testing.T) {
	src := &models.Scene{ID: 1, OSHash: "1111111111111111"}
	p := mergeMarker(t, src, 2, "")

	for _, f := range markerFiles(p, src.OSHash) {
		require.FileExists(t, f, "not moved to a scene without a hash")
	}
}
