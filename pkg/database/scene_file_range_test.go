//go:build integration

package database_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/plugin"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rangeVideoFile(ctx context.Context, t *testing.T, name string, duration float64) *models.VideoFile {
	t.Helper()
	f := &models.VideoFile{
		BaseFile: &models.BaseFile{
			Basename:       name,
			ParentFolderID: folderIDs[folderIdxWithSceneFiles],
			Size:           100,
			DirEntry:       models.DirEntry{ModTime: time.Now()},
			Fingerprints: models.Fingerprints{
				{Type: models.FingerprintTypeOshash, Fingerprint: "oshash-" + name},
			},
		},
		Format:     "mp4",
		Duration:   duration,
		VideoCodec: "h264",
		Width:      640,
		Height:     480,
		FrameRate:  30,
		BitRate:    1000,
	}
	require.NoError(t, db.File().Create(ctx, f))
	return f
}

func rangeService() *scene.Service {
	return &scene.Service{
		File:             db.File(),
		Repository:       db.Scene(),
		MarkerRepository: db.SceneMarker(),
		PluginCache:      &plugin.Cache{},
	}
}

func f64(v float64) *float64 { return &v }

func newRangeScene(ctx context.Context, t *testing.T, svc *scene.Service, title string, start, end *float64, fileIDs ...models.FileID) (*models.Scene, error) {
	t.Helper()
	s := models.NewScene()
	s.Title = title
	s.StartTime = start
	s.EndTime = end
	return svc.Create(ctx, models.CreateSceneInput{Scene: &s, FileIDs: fileIDs})
}

func TestSceneFileRanges(t *testing.T) {
	runWithRollbackTxn(t, "several scenes share a file", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		f := rangeVideoFile(ctx, t, "compilation.mp4", 600)
		other := rangeVideoFile(ctx, t, "other.mp4", 600)

		whole, err := newRangeScene(ctx, t, svc, "whole", nil, nil, f.ID)
		require.NoError(t, err)
		first, err := newRangeScene(ctx, t, svc, "first", f64(0), f64(100), f.ID)
		require.NoError(t, err)
		second, err := newRangeScene(ctx, t, svc, "second", f64(100), nil, f.ID)
		require.NoError(t, err)

		got, err := db.Scene().Find(ctx, first.ID)
		require.NoError(t, err)
		assert.Equal(t, f64(0), got.StartTime)
		assert.Equal(t, f64(100), got.EndTime)
		assert.Equal(t, &f.ID, got.PrimaryFileID)
		got, err = db.Scene().Find(ctx, second.ID)
		require.NoError(t, err)
		assert.Equal(t, f64(100), got.StartTime)
		assert.Nil(t, got.EndTime)

		// the whole-file scene keeps its file and has no range
		got, err = db.Scene().Find(ctx, whole.ID)
		require.NoError(t, err)
		assert.Equal(t, &f.ID, got.PrimaryFileID)
		assert.Nil(t, got.StartTime)
		assert.Nil(t, got.EndTime)

		scenes, err := db.Scene().FindByFileID(ctx, f.ID)
		require.NoError(t, err)
		assert.Len(t, scenes, 3)

		// only one scene can use the whole file
		_, err = newRangeScene(ctx, t, svc, "another whole", nil, nil, f.ID)
		require.Error(t, err)
		require.NoError(t, first.LoadFiles(ctx, db.Scene()))
		assert.ErrorIs(t, svc.ValidateRangeUpdate(ctx, first, nil, nil), scene.ErrWholeFileInUse)

		// a part of a file has only that file
		_, err = newRangeScene(ctx, t, svc, "two files", f64(1), f64(2), f.ID, other.ID)
		assert.ErrorIs(t, err, scene.ErrRangeNeedsOneFile)
		assert.ErrorIs(t, svc.AssignFile(ctx, first.ID, other.ID), scene.ErrRangeNeedsOneFile)
		lone, err := newRangeScene(ctx, t, svc, "lone", nil, nil, other.ID)
		require.NoError(t, err)
		extra := rangeVideoFile(ctx, t, "extra.mp4", 600)
		require.NoError(t, svc.AssignFile(ctx, lone.ID, extra.ID))
		require.NoError(t, lone.LoadFiles(ctx, db.Scene()))
		assert.ErrorIs(t, svc.ValidateRangeUpdate(ctx, lone, f64(1), f64(2)), scene.ErrRangeNeedsOneFile)

		// a shared file can't be taken by another scene
		assert.Error(t, svc.AssignFile(ctx, lone.ID, f.ID))
		scenes, err = db.Scene().FindByFileID(ctx, f.ID)
		require.NoError(t, err)
		assert.Len(t, scenes, 3)

		soloFile := rangeVideoFile(ctx, t, "solo.mp4", 600)
		solo, err := newRangeScene(ctx, t, svc, "solo", f64(1), f64(2), soloFile.ID)
		require.NoError(t, err)

		// merging would move the shared file away from the other scenes, or lose the range
		for _, m := range []struct {
			name    string
			sources []int
			dest    int
		}{
			{"a part into the whole", []int{first.ID}, whole.ID},
			{"the whole into another scene", []int{whole.ID}, lone.ID},
			{"another scene into a part", []int{lone.ID}, first.ID},
			{"a part with a file to itself", []int{solo.ID}, lone.ID},
		} {
			err := svc.Merge(ctx, m.sources, m.dest, nil, scene.MergeOptions{})
			assert.ErrorIs(t, err, scene.ErrMergeSharedFile, m.name)
		}
		scenes, err = db.Scene().FindByFileID(ctx, f.ID)
		require.NoError(t, err)
		assert.Len(t, scenes, 3)
	})

	runWithRollbackTxn(t, "ranges are checked against the file", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		f := rangeVideoFile(ctx, t, "checked.mp4", 60)

		for _, c := range []struct {
			name       string
			start, end *float64
		}{
			{"not a number", f64(math.NaN()), nil},
			{"endless", nil, f64(math.Inf(1))},
			{"before the start", f64(-1), nil},
			{"ends at the start", nil, f64(0)},
			{"starts after it ends", f64(30), f64(20)},
			{"empty", f64(30), f64(30)},
			{"starts after the file", f64(60), nil},
			{"ends after the file", nil, f64(61)},
		} {
			_, err := newRangeScene(ctx, t, svc, c.name, c.start, c.end, f.ID)
			assert.Error(t, err, c.name)
		}
		_, err := newRangeScene(ctx, t, svc, "no file", f64(1), f64(2))
		assert.ErrorIs(t, err, scene.ErrRangeNeedsOneFile)

		_, err = newRangeScene(ctx, t, svc, "the whole length", f64(0), f64(60), f.ID)
		assert.NoError(t, err)

		// a file that wasn't read properly has no duration to check against
		unread := rangeVideoFile(ctx, t, "unread.mp4", 0)
		_, err = newRangeScene(ctx, t, svc, "unread", f64(10), f64(20), unread.ID)
		assert.NoError(t, err)
	})

	runWithRollbackTxn(t, "a range can be changed and cleared", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		f := rangeVideoFile(ctx, t, "changed.mp4", 600)
		part, err := newRangeScene(ctx, t, svc, "part", f64(10), f64(20), f.ID)
		require.NoError(t, err)

		partial := models.NewScenePartial()
		partial.EndTime = models.NewOptionalFloat64(30)
		got, err := db.Scene().UpdatePartial(ctx, part.ID, partial)
		require.NoError(t, err)
		assert.Equal(t, f64(10), got.StartTime)
		assert.Equal(t, f64(30), got.EndTime)

		// a whole-file scene can be made when only parts of the file are used
		whole, err := newRangeScene(ctx, t, svc, "whole", nil, nil, f.ID)
		require.NoError(t, err)
		got, err = db.Scene().Find(ctx, part.ID)
		require.NoError(t, err)
		assert.Equal(t, &f.ID, got.PrimaryFileID)

		// with the whole-file scene removed, the part can become the whole file
		require.NoError(t, db.Scene().Destroy(ctx, whole.ID))
		require.NoError(t, svc.ValidateRangeUpdate(ctx, got, nil, nil))
		partial = models.NewScenePartial()
		partial.StartTime = models.NewOptionalFloat64Ptr(nil)
		partial.EndTime = models.NewOptionalFloat64Ptr(nil)
		got, err = db.Scene().UpdatePartial(ctx, part.ID, partial)
		require.NoError(t, err)
		assert.Nil(t, got.StartTime)
		assert.Nil(t, got.EndTime)
	})

	runWithRollbackTxn(t, "replacing the files keeps the range", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		f := rangeVideoFile(ctx, t, "replaced.mp4", 600)
		part, err := newRangeScene(ctx, t, svc, "part", f64(5), f64(50), f.ID)
		require.NoError(t, err)

		// importing a scene replaces its files this way
		require.NoError(t, part.LoadRelationships(ctx, db.Scene()))
		part.Title = "imported"
		require.NoError(t, db.Scene().Update(ctx, part))

		got, err := db.Scene().Find(ctx, part.ID)
		require.NoError(t, err)
		assert.Equal(t, "imported", got.Title)
		assert.Equal(t, f64(5), got.StartTime)
		assert.Equal(t, f64(50), got.EndTime)
	})

	runWithRollbackTxn(t, "removing a scene keeps what other scenes on the file need", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		f := rangeVideoFile(ctx, t, "removed.mp4", 600)
		whole, err := newRangeScene(ctx, t, svc, "whole", nil, nil, f.ID)
		require.NoError(t, err)
		part, err := newRangeScene(ctx, t, svc, "part", f64(10), f64(20), f.ID)
		require.NoError(t, err)

		generated := t.TempDir()
		p := paths.NewPaths(generated, "")
		require.NoError(t, os.MkdirAll(filepath.Dir(p.Scene.GetSpriteImageFilePath(part.OSHash)), 0o755))
		sprite := p.Scene.GetSpriteImageFilePath(part.OSHash)
		require.NoError(t, os.WriteFile(sprite, []byte("sprite"), 0o644))

		// both scenes have a marker at the same time, which share a preview
		for _, id := range []int{whole.ID, part.ID} {
			m := &models.SceneMarker{Title: "marker", SceneID: id, PrimaryTagID: tagIDs[tagIdxWithScene], Seconds: 15}
			require.NoError(t, db.SceneMarker().Create(ctx, m))
		}
		preview := p.SceneMarkers.GetWebpPreviewPath(part.OSHash, 15)
		require.NoError(t, os.MkdirAll(filepath.Dir(preview), 0o755))
		require.NoError(t, os.WriteFile(preview, []byte("preview"), 0o644))

		deleter := &scene.FileDeleter{Deleter: file.NewDeleter(), FileNamingAlgo: models.HashAlgorithmOshash, Paths: &p}
		const deleteGenerated, deleteFile, destroyFileEntry = true, true, false
		require.NoError(t, svc.Destroy(ctx, part, deleter, deleteGenerated, deleteFile, destroyFileEntry))
		deleter.Commit()

		assert.FileExists(t, sprite, "the other scene's sprite is kept")
		assert.FileExists(t, preview, "the other scene's marker preview is kept")
		markers, err := db.SceneMarker().FindBySceneID(ctx, part.ID)
		require.NoError(t, err)
		assert.Empty(t, markers, "the removed scene's markers are removed")
		files, err := db.File().Find(ctx, f.ID)
		require.NoError(t, err)
		assert.Len(t, files, 1, "the file is kept")
		got, err := db.Scene().Find(ctx, whole.ID)
		require.NoError(t, err)
		assert.Equal(t, &f.ID, got.PrimaryFileID)

		// the last scene on the file removes them as before
		require.NoError(t, svc.Destroy(ctx, got, deleter, deleteGenerated, false, false))
		deleter.Commit()
		assert.NoFileExists(t, sprite)
		assert.NoFileExists(t, preview)
	})
}
