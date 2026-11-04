//go:build integration

package database_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stretchr/testify/require"
)

type oshashNaming struct{}

func (oshashNaming) GetVideoFileNamingAlgorithm() models.HashAlgorithm {
	return models.HashAlgorithmOshash
}

// #2038: merging scenes moves every marker's generated files to the hash of
// the kept scene, including a kept scene that had no file of its own and
// takes the first source's.
func TestMergeMovesMarkerFilesToTheKeptScene(t *testing.T) {
	for _, keptHasFile := range []bool{true, false} {
		name := map[bool]string{true: "kept scene with a file", false: "kept scene without a file"}[keptHasFile]
		t.Run(name, func(t *testing.T) {
			prefix := map[bool]string{true: "merge-markers-file-", false: "merge-markers-nofile-"}[keptHasFile]
			hash := func(n string) string { return "oshash-" + prefix + n + ".mp4" }

			p := paths.NewPaths(t.TempDir(), "")
			svc := rangeService()
			svc.Paths = &p
			svc.Config = oshashNaming{}

			now := time.Now()
			var a, b, kept *models.Scene
			var fileIDs []models.FileID
			newScene := func(ctx context.Context, title string, withFile bool) *models.Scene {
				s := &models.Scene{Title: prefix + title, CreatedAt: now, UpdatedAt: now}
				var ids []models.FileID
				if withFile {
					f := rangeVideoFile(ctx, t, prefix+title+".mp4", 60)
					ids = append(ids, f.ID)
					fileIDs = append(fileIDs, f.ID)
				}
				require.NoError(t, db.Scene().Create(ctx, s, ids))
				return s
			}
			require.NoError(t, withTxn(func(ctx context.Context) error {
				a = newScene(ctx, "a", true)
				b = newScene(ctx, "b", true)
				kept = newScene(ctx, "kept", keptHasFile)
				for _, m := range []*models.SceneMarker{
					{Title: "a", SceneID: a.ID, PrimaryTagID: tagIDs[tagIdxWithScene], Seconds: 5, CreatedAt: now, UpdatedAt: now},
					{Title: "b", SceneID: b.ID, PrimaryTagID: tagIDs[tagIdxWithScene], Seconds: 7, CreatedAt: now, UpdatedAt: now},
				} {
					if err := db.SceneMarker().Create(ctx, m); err != nil {
						return err
					}
				}
				return nil
			}))
			t.Cleanup(func() {
				// the merge has destroyed a and b
				require.NoError(t, withTxn(func(ctx context.Context) error {
					markers, err := db.SceneMarker().FindBySceneID(ctx, kept.ID)
					if err != nil {
						return err
					}
					for _, m := range markers {
						if err := db.SceneMarker().Destroy(ctx, m.ID); err != nil {
							return err
						}
					}
					if err := db.Scene().Destroy(ctx, kept.ID); err != nil {
						return err
					}
					for _, id := range fileIDs {
						if err := db.File().Destroy(ctx, id); err != nil {
							return err
						}
					}
					return nil
				}))
			})

			markerFiles := func(h string, seconds int) []string {
				return []string{
					p.SceneMarkers.GetVideoPreviewPath(h, seconds),
					p.SceneMarkers.GetWebpPreviewPath(h, seconds),
					p.SceneMarkers.GetScreenshotPath(h, seconds),
				}
			}
			for _, f := range append(markerFiles(hash("a"), 5), markerFiles(hash("b"), 7)...) {
				require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o755))
				require.NoError(t, os.WriteFile(f, []byte("generated"), 0o644))
			}

			deleter := &scene.FileDeleter{Deleter: file.NewDeleter(), FileNamingAlgo: models.HashAlgorithmOshash, Paths: &p}
			require.NoError(t, withTxn(func(ctx context.Context) error {
				return svc.Merge(ctx, []int{a.ID, b.ID}, kept.ID, deleter, scene.MergeOptions{})
			}))
			deleter.Commit()

			// without a file of its own, the kept scene takes a's
			want := hash("a")
			if keptHasFile {
				want = hash("kept")
			}
			for _, f := range append(markerFiles(want, 5), markerFiles(want, 7)...) {
				require.FileExists(t, f, "under the kept scene's hash")
			}
		})
	}
}
