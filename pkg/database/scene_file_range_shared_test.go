//go:build integration

package database_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/plugin"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The things that several scenes sharing one file must not break: backups, scans,
// library limits, merges, markers, duplicates, totals and stash-box.

// sharedFile makes a 600-second file with a whole-file scene and three parts, two of
// them untitled.
func sharedFile(ctx context.Context, t *testing.T, svc *scene.Service, name string, fps models.Fingerprints) (*models.VideoFile, []*models.Scene) {
	t.Helper()
	f := rangeVideoFileWith(ctx, t, name, 600, fps)
	var scenes []*models.Scene
	for _, c := range []struct {
		title      string
		start, end *float64
	}{
		{"whole " + name, nil, nil},
		{"part " + name, f64(0), f64(100)},
		{"", f64(100), f64(300)},
		{"", f64(300), nil},
	} {
		s, err := newRangeScene(ctx, t, svc, c.title, c.start, c.end, f.ID)
		require.NoError(t, err, c.title)
		scenes = append(scenes, s)
	}
	return f, scenes
}

func sceneRanges(ctx context.Context, t *testing.T, fileID models.FileID) map[string]bool {
	t.Helper()
	scenes, err := db.Scene().FindByFileID(ctx, fileID)
	require.NoError(t, err)
	ret := map[string]bool{}
	for _, s := range scenes {
		ret[s.Title+" "+rangeString(s.StartTime)+"-"+rangeString(s.EndTime)] = true
	}
	return ret
}

func rangeString(v *float64) string {
	if v == nil {
		return "_"
	}
	return time.Duration(*v * float64(time.Second)).String()
}

func importScene(ctx context.Context, t *testing.T, in jsonschema.Scene) int {
	t.Helper()
	i := &scene.Importer{
		ReaderWriter:        db.Scene(),
		FileFinder:          db.File(),
		StudioWriter:        db.Studio(),
		GalleryFinder:       db.Gallery(),
		PerformerWriter:     db.Performer(),
		GroupWriter:         db.Group(),
		TagWriter:           db.Tag(),
		VoiceActorWriter:    db.VoiceActor(),
		Input:               in,
		MissingRefBehaviour: models.ImportMissingRefEnumFail,
		FileNamingAlgorithm: models.HashAlgorithmOshash,
	}
	require.NoError(t, i.PreImport(ctx))
	id, err := i.FindExistingID(ctx)
	require.NoError(t, err)
	if id == nil {
		id, err = i.Create(ctx)
		require.NoError(t, err)
	} else {
		require.NoError(t, i.Update(ctx, *id))
	}
	require.NoError(t, i.PostImport(ctx, *id))
	return *id
}

func tempPaths(t *testing.T) *paths.Paths {
	p := paths.NewPaths(t.TempDir(), "")
	return &p
}

type noGenerate struct{}

func (noGenerate) Generate(ctx context.Context, s *models.Scene, f *models.VideoFile) error {
	return nil
}

func TestSceneFileRangesShared(t *testing.T) {
	runWithRollbackTxn(t, "a backup and restore keeps every scene on the file", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		f, scenes := sharedFile(ctx, t, svc, "backup.mp4", models.Fingerprints{
			{Type: models.FingerprintTypeOshash, Fingerprint: "oshash-backup"},
		})
		want := sceneRanges(ctx, t, f.ID)
		require.Len(t, want, 4)

		var exported []jsonschema.Scene
		names := map[string]bool{}
		for _, s := range scenes {
			got, err := db.Scene().Find(ctx, s.ID)
			require.NoError(t, err)
			require.NoError(t, got.LoadRelationships(ctx, db.Scene()))
			j, err := scene.ToBasicJSON(ctx, db.Scene(), got)
			require.NoError(t, err)
			exported = append(exported, *j)
			names[j.Filename(got.ID, filepath.Base(got.Path), got.OSHash)] = true
		}
		assert.Len(t, names, 4, "each scene has its own export file")

		// a full import into an empty library
		for _, s := range scenes {
			require.NoError(t, db.Scene().Destroy(ctx, s.ID))
		}
		require.Empty(t, sceneRanges(ctx, t, f.ID))
		for _, j := range exported {
			importScene(ctx, t, j)
		}
		assert.Equal(t, want, sceneRanges(ctx, t, f.ID))

		// importing again over the restored scenes changes nothing
		for _, j := range exported {
			importScene(ctx, t, j)
		}
		assert.Equal(t, want, sceneRanges(ctx, t, f.ID))
	})

	runWithRollbackTxn(t, "a copy of the file only joins the whole-file scene", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		fps := models.Fingerprints{{Type: models.FingerprintTypeOshash, Fingerprint: "oshash-copied"}}
		_, scenes := sharedFile(ctx, t, svc, "copied.mp4", fps)
		cp := rangeVideoFileWith(ctx, t, "copied-copy.mp4", 600, fps)

		h := &scene.ScanHandler{
			CreatorUpdater:       db.Scene(),
			GalleryFinderUpdater: db.Gallery(),
			CaptionUpdater:       db.File(),
			ScanGenerator:        noGenerate{},
			PluginCache:          &plugin.Cache{},
			FileNamingAlgorithm:  models.HashAlgorithmOshash,
			Paths:                tempPaths(t),
		}
		require.NoError(t, h.Handle(ctx, cp, nil))

		for i, s := range scenes {
			files, err := db.Scene().GetFiles(ctx, s.ID)
			require.NoError(t, err)
			if i == 0 {
				assert.Len(t, files, 2, "the whole-file scene gains the copy")
			} else {
				assert.Len(t, files, 1, "a part keeps only its file")
			}
		}
	})

	runWithRollbackTxn(t, "the checks see scenes the account can't", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		f, scenes := sharedFile(ctx, t, svc, "limited.mp4", nil)
		part := scenes[1]
		limited := models.WithLibraryLimit(ctx, &models.LibraryLimit{})

		visible, err := db.Scene().FindByFileID(limited, f.ID)
		require.NoError(t, err)
		require.Empty(t, visible, "the limit hides the scenes")

		_, _, err = svc.ValidateRangeUpdate(limited, part, nil, nil)
		assert.ErrorIs(t, err, scene.ErrWholeFileInUse)
		_, err = newRangeScene(limited, t, svc, "another whole", f64(0), nil, f.ID)
		assert.Error(t, err)

		deleter := &scene.FileDeleter{Deleter: file.NewDeleter(), FileNamingAlgo: models.HashAlgorithmOshash, Paths: tempPaths(t)}
		const deleteGenerated, deleteFile, destroyFileEntry = true, true, false
		require.NoError(t, svc.Destroy(limited, part, deleter, deleteGenerated, deleteFile, destroyFileEntry))
		deleter.Commit()
		files, err := db.File().Find(ctx, f.ID)
		require.NoError(t, err)
		assert.Len(t, files, 1, "the file the other scenes use is kept")
		assert.Len(t, sceneRanges(ctx, t, f.ID), 3)
	})

	runWithRollbackTxn(t, "a range that covers the whole file is the whole file", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		f, scenes := sharedFile(ctx, t, svc, "covered.mp4", nil)

		for _, c := range []struct {
			name       string
			start, end *float64
		}{
			{"from the start, with no end", f64(0), nil},
			{"from the start to the end", f64(0), f64(600)},
			{"with no start, to the end", nil, f64(600)},
		} {
			_, err := newRangeScene(ctx, t, svc, c.name, c.start, c.end, f.ID)
			assert.ErrorIs(t, err, scene.ErrWholeFileInUse, c.name)
			_, _, err = svc.ValidateRangeUpdate(ctx, scenes[1], c.start, c.end)
			assert.ErrorIs(t, err, scene.ErrWholeFileInUse, c.name)
		}

		// on a file nobody uses, it's stored as the whole file
		alone := rangeVideoFile(ctx, t, "covered-alone.mp4", 600)
		s, err := newRangeScene(ctx, t, svc, "covers it all", f64(0), f64(600), alone.ID)
		require.NoError(t, err)
		assert.Nil(t, s.StartTime)
		assert.Nil(t, s.EndTime)

		// a part's start can still be moved to the start of the file
		start, end, err := svc.ValidateRangeUpdate(ctx, scenes[2], f64(0), f64(300))
		require.NoError(t, err)
		assert.Nil(t, start)
		assert.Equal(t, f64(300), end)
	})

	runWithRollbackTxn(t, "a merge can't set times", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		a, err := newRangeScene(ctx, t, svc, "merge a", nil, nil, rangeVideoFile(ctx, t, "merge-a.mp4", 60).ID)
		require.NoError(t, err)
		b, err := newRangeScene(ctx, t, svc, "merge b", nil, nil, rangeVideoFile(ctx, t, "merge-b.mp4", 60).ID)
		require.NoError(t, err)

		values := models.NewScenePartial()
		values.StartTime = models.NewOptionalFloat64(-1)
		values.EndTime = models.NewOptionalFloat64(-1)
		err = svc.Merge(ctx, []int{b.ID}, a.ID, nil, scene.MergeOptions{ScenePartial: values})
		assert.ErrorIs(t, err, scene.ErrMergeRange)

		got, err := db.Scene().Find(ctx, a.ID)
		require.NoError(t, err)
		assert.Nil(t, got.StartTime)
		files, err := db.Scene().GetFiles(ctx, a.ID)
		require.NoError(t, err)
		assert.Len(t, files, 1)
	})

	runWithRollbackTxn(t, "a marker's files stay while another marker uses them", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		_, scenes := sharedFile(ctx, t, svc, "markers.mp4", models.Fingerprints{
			{Type: models.FingerprintTypeOshash, Fingerprint: "oshash-markers"},
		})
		whole, part := scenes[0], scenes[1]

		p := paths.NewPaths(t.TempDir(), "")
		var markers []*models.SceneMarker
		for _, id := range []int{whole.ID, part.ID} {
			m := &models.SceneMarker{Title: "marker", SceneID: id, PrimaryTagID: tagIDs[tagIdxWithScene], Seconds: 30}
			require.NoError(t, db.SceneMarker().Create(ctx, m))
			markers = append(markers, m)
		}
		generated := []string{
			p.SceneMarkers.GetVideoPreviewPath(whole.OSHash, 30),
			p.SceneMarkers.GetWebpPreviewPath(whole.OSHash, 30),
			p.SceneMarkers.GetScreenshotPath(whole.OSHash, 30),
		}
		for _, g := range generated {
			require.NoError(t, os.MkdirAll(filepath.Dir(g), 0o755))
			require.NoError(t, os.WriteFile(g, []byte("generated"), 0o644))
		}

		deleter := &scene.FileDeleter{Deleter: file.NewDeleter(), FileNamingAlgo: models.HashAlgorithmOshash, Paths: &p}
		require.NoError(t, svc.DestroyMarker(ctx, part, markers[1], deleter))
		deleter.Commit()
		for _, g := range generated {
			assert.FileExists(t, g, "the whole-file scene's marker still uses it")
		}

		require.NoError(t, svc.DestroyMarker(ctx, whole, markers[0], deleter))
		deleter.Commit()
		for _, g := range generated {
			assert.NoFileExists(t, g, "the last marker using it takes it")
		}
	})

	runWithRollbackTxn(t, "parts of a file aren't duplicates of each other", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		phash := models.Fingerprint{Type: models.FingerprintTypePhash, Fingerprint: int64(0x1234567890ab)}
		_, scenes := sharedFile(ctx, t, svc, "dupes.mp4", models.Fingerprints{phash})
		copyFile := rangeVideoFileWith(ctx, t, "dupes-copy.mp4", 600, models.Fingerprints{phash})
		copyScene, err := newRangeScene(ctx, t, svc, "copy", nil, nil, copyFile.ID)
		require.NoError(t, err)

		for _, distance := range []int{0, 1} {
			groups, err := db.Scene().FindDuplicates(ctx, distance, -1, nil)
			require.NoError(t, err)
			var found []int
			for _, g := range groups {
				for _, s := range g {
					if s.ID == scenes[0].ID || s.ID == copyScene.ID {
						found = append(found, s.ID)
					}
					for _, p := range scenes[1:] {
						assert.NotEqual(t, p.ID, s.ID, "a part is listed as a duplicate at distance %d", distance)
					}
				}
			}
			slices.Sort(found)
			assert.Equal(t, []int{scenes[0].ID, copyScene.ID}, found, "the real copy is still found at distance %d", distance)
		}
	})

	runWithRollbackTxn(t, "totals count a shared file once and a part by its length", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		sizeBefore, err := db.Scene().Size(ctx)
		require.NoError(t, err)
		durationBefore, err := db.Scene().Duration(ctx)
		require.NoError(t, err)

		// a 600-second file of size 100: the whole of it, and parts of 100, 200 and 300 seconds
		sharedFile(ctx, t, svc, "totals.mp4", nil)

		size, err := db.Scene().Size(ctx)
		require.NoError(t, err)
		assert.Equal(t, float64(100), size-sizeBefore)
		duration, err := db.Scene().Duration(ctx)
		require.NoError(t, err)
		assert.InDelta(t, 1200, duration-durationBefore, 0.001)

		title := "totals.mp4"
		r, err := db.Scene().Query(ctx, models.SceneQueryOptions{
			QueryOptions:  models.QueryOptions{Count: true},
			SceneFilter:   &models.SceneFilterType{Title: &models.StringCriterionInput{Value: title, Modifier: models.CriterionModifierIncludes}},
			TotalDuration: true,
			TotalSize:     true,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, r.Count, "the titled scenes")
		assert.Equal(t, float64(100), r.TotalSize)
		assert.InDelta(t, 700, r.TotalDuration, 0.001)
	})

	runWithRollbackTxn(t, "a part has no fingerprints of its own", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		_, scenes := sharedFile(ctx, t, svc, "stashbox.mp4", models.Fingerprints{
			{Type: models.FingerprintTypeOshash, Fingerprint: "oshash-stashbox"},
		})
		var ids []int
		for _, s := range scenes {
			ids = append(ids, s.ID)
		}
		fps, err := svc.GetScenesFingerprints(ctx, ids)
		require.NoError(t, err)
		require.Len(t, fps, 4)
		assert.Len(t, fps[0], 1, "the whole-file scene is looked up by its file")
		for _, fp := range fps[1:] {
			assert.Empty(t, fp, "a part isn't looked up as the whole file")
		}
	})
}

// Two changes at once on PostgreSQL can't both make a scene use the whole of a file.
// SQLite runs one write at a time.
func TestSceneWholeFileAtOnce(t *testing.T) {
	if IsPostgresTest() == nil {
		t.Skip("only PostgreSQL runs two writes at once")
	}

	svc := rangeService()
	var f *models.VideoFile
	var a, b *models.Scene
	// made in the store, since a committed scene create runs plugin hooks
	require.NoError(t, withTxn(func(ctx context.Context) error {
		f = rangeVideoFile(ctx, t, "at-once.mp4", 600)
		for _, s := range []**models.Scene{&a, &b} {
			n := models.NewScene()
			n.Title = "at once"
			n.StartTime, n.EndTime = f64(10), f64(20)
			if s == &b {
				n.StartTime, n.EndTime = f64(20), f64(30)
			}
			if err := db.Scene().Create(ctx, &n, []models.FileID{f.ID}); err != nil {
				return err
			}
			*s = &n
		}
		return nil
	}))
	t.Cleanup(func() {
		_ = withTxn(func(ctx context.Context) error {
			for _, s := range []*models.Scene{a, b} {
				if err := db.Scene().Destroy(ctx, s.ID); err != nil {
					return err
				}
			}
			return db.File().Destroy(ctx, f.ID)
		})
	})

	makeWhole := func(ctx context.Context, id int) error {
		s, err := db.Scene().Find(ctx, id)
		if err != nil {
			return err
		}
		if _, _, err := svc.ValidateRangeUpdate(ctx, s, nil, nil); err != nil {
			return err
		}
		partial := models.NewScenePartial()
		partial.StartTime = models.NewOptionalFloat64Ptr(nil)
		partial.EndTime = models.NewOptionalFloat64Ptr(nil)
		_, err = db.Scene().UpdatePartial(ctx, id, partial)
		return err
	}

	checked := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- withTxn(func(ctx context.Context) error {
			err := makeWhole(ctx, a.ID)
			close(checked)
			<-release
			return err
		})
	}()
	<-checked

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- withTxn(func(ctx context.Context) error {
			return makeWhole(ctx, b.ID)
		})
	}()

	select {
	case err := <-secondDone:
		close(release)
		<-firstDone
		t.Fatalf("the second change finished while the first was still open: %v", err)
	case <-time.After(time.Second):
	}

	close(release)
	require.NoError(t, <-firstDone)
	assert.ErrorIs(t, <-secondDone, scene.ErrWholeFileInUse)
}
