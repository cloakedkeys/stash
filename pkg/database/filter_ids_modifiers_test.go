//go:build integration
// +build integration

package database_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2235: chosen ids must be passed to the query as values. An id that isn't
// a number must not change the query: it is refused with "invalid id".
func TestFilterIDsNotInQueryText(t *testing.T) {
	refusedOrEmpty := func(t *testing.T, n int, err error) {
		t.Helper()
		assert.ErrorContains(t, err, "invalid id")
		assert.Zero(t, n, "an id that isn't a number must not match anything")
	}

	runWithRollbackTxn(t, "performers by the performers they appear with", func(t *testing.T, ctx context.Context) {
		var ids []int
		for _, n := range []string{"Q", "R", "with Q", "with R"} {
			p := models.Performer{Name: "id text " + n}
			require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &p}))
			ids = append(ids, p.ID)
		}
		for _, pair := range [][2]int{{ids[2], ids[0]}, {ids[3], ids[1]}} {
			s := models.Scene{Title: "id text scene", PerformerIDs: models.NewRelatedIDs([]int{pair[0], pair[1]})}
			require.NoError(t, db.Scene().Create(ctx, &s, nil))
		}

		// one "id" that would read as two if written into the query
		value := fmt.Sprintf("%d),(%d", ids[0], ids[1])
		performers, _, err := db.Performer().Query(ctx, &models.PerformerFilterType{Performers: &models.MultiCriterionInput{
			Value: []string{value}, Modifier: models.CriterionModifierIncludes,
		}}, perPageAll())
		refusedOrEmpty(t, len(performers), err)
	})

	runWithRollbackTxn(t, "scenes by tags", func(t *testing.T, ctx context.Context) {
		tg := models.Tag{Name: "id text tag"}
		require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: &tg}))
		s := models.Scene{Title: "id text tagged scene", TagIDs: models.NewRelatedIDs([]int{tg.ID})}
		require.NoError(t, db.Scene().Create(ctx, &s, nil))

		// an "id" that is a query of its own, finding the newest tag
		for _, depth := range []int{0, -1} {
			d := depth
			res, err := db.Scene().Query(ctx, models.SceneQueryOptions{
				QueryOptions: models.QueryOptions{FindFilter: perPageAll()},
				SceneFilter: &models.SceneFilterType{Tags: &models.HierarchicalMultiCriterionInput{
					Value: []string{"(SELECT MAX(id) FROM tags)"}, Modifier: models.CriterionModifierIncludes, Depth: &d,
				}},
			})
			n := 0
			if res != nil {
				n = len(res.IDs)
			}
			refusedOrEmpty(t, n, err)
		}
	})
}

// #2235: the performer Groups filter skipped itself on PostgreSQL when an id
// wasn't a number, and matched nothing on SQLite. Both now refuse it with the
// same error as the other filters.
func TestPerformerGroupsNonNumericID(t *testing.T) {
	for _, depth := range []int{0, -1} {
		d := depth
		runWithRollbackTxn(t, "depth "+strconv.Itoa(d), func(t *testing.T, ctx context.Context) {
			_, _, err := db.Performer().Query(ctx, &models.PerformerFilterType{Groups: &models.HierarchicalMultiCriterionInput{
				Value: []string{"not a number"}, Modifier: models.CriterionModifierIncludes, Depth: &d,
			}}, perPageAll())
			assert.ErrorContains(t, err, "invalid id")
		})
	}
}

// #2235: a filter that doesn't handle a modifier refuses it with an error,
// as the gallery parent folder filter refuses "includes all", instead of
// ignoring it and returning everything.
func TestFilterUnsupportedModifiersRefused(t *testing.T) {
	one := []string{"1"}
	performerFilter := func(f *models.PerformerFilterType) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			_, _, err := db.Performer().Query(ctx, f, perPageAll())
			return err
		}
	}
	sceneFilter := func(f *models.SceneFilterType) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			_, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll()}, SceneFilter: f})
			return err
		}
	}
	multi := func(m models.CriterionModifier) *models.MultiCriterionInput {
		return &models.MultiCriterionInput{Value: one, Modifier: m}
	}
	hier := func(m models.CriterionModifier) *models.HierarchicalMultiCriterionInput {
		return &models.HierarchicalMultiCriterionInput{Value: one, Modifier: m}
	}

	tests := []struct {
		name  string
		query func(ctx context.Context) error
	}{
		{"performers by studios, equals", performerFilter(&models.PerformerFilterType{Studios: hier(models.CriterionModifierEquals)})},
		{"performers by groups, equals", performerFilter(&models.PerformerFilterType{Groups: hier(models.CriterionModifierEquals)})},
		{"performers by performers, equals", performerFilter(&models.PerformerFilterType{Performers: multi(models.CriterionModifierEquals)})},
		{"performers by performers, excludes", performerFilter(&models.PerformerFilterType{Performers: multi(models.CriterionModifierExcludes)})},
		{"performers by performers, is null", performerFilter(&models.PerformerFilterType{Performers: &models.MultiCriterionInput{Modifier: models.CriterionModifierIsNull}})},
		{"performers by performers, not null", performerFilter(&models.PerformerFilterType{Performers: &models.MultiCriterionInput{Modifier: models.CriterionModifierNotNull}})},
		{"groups by performers, equals", func(ctx context.Context) error {
			_, _, err := db.Group().Query(ctx, &models.GroupFilterType{Performers: multi(models.CriterionModifierEquals)}, perPageAll())
			return err
		}},
		// the shared multi-item code
		{"scenes by galleries, equals", sceneFilter(&models.SceneFilterType{Galleries: multi(models.CriterionModifierEquals)})},
		// the shared joined multi-item code
		{"scenes by performers, greater than", sceneFilter(&models.SceneFilterType{Performers: multi(models.CriterionModifierGreaterThan)})},
		// the shared hierarchical code
		{"scenes by studios, greater than", sceneFilter(&models.SceneFilterType{Studios: hier(models.CriterionModifierGreaterThan)})},
		{"scenes by tags, greater than", sceneFilter(&models.SceneFilterType{Tags: hier(models.CriterionModifierGreaterThan)})},
		{"files by zip file, includes all", func(ctx context.Context) error {
			_, err := db.File().Query(ctx, models.FileQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll()}, FileFilter: &models.FileFilterType{ZipFile: multi(models.CriterionModifierIncludesAll)}})
			return err
		}},
		{"folders by zip file, includes all", func(ctx context.Context) error {
			_, err := db.Folder().Query(ctx, models.FolderQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll()}, FolderFilter: &models.FolderFilterType{ZipFile: multi(models.CriterionModifierIncludesAll)}})
			return err
		}},
	}

	for _, tt := range tests {
		runWithRollbackTxn(t, tt.name, func(t *testing.T, ctx context.Context) {
			assert.ErrorContains(t, tt.query(ctx), "invalid modifier")
		})
	}
}

// #2235: the zip file filter ignored the excludes list.
func TestFileZipFileExcludesList(t *testing.T) {
	runWithRollbackTxn(t, "files by zip file", func(t *testing.T, ctx context.Context) {
		root := &models.Folder{Path: filepath.Join(t.TempDir(), "zip excludes")}
		require.NoError(t, db.Folder().Create(ctx, root))
		newFile := func(name string, zip *models.FileID) models.FileID {
			f := &models.BaseFile{ParentFolderID: root.ID, Basename: name, DirEntry: models.DirEntry{ZipFileID: zip}}
			require.NoError(t, db.File().Create(ctx, f))
			return f.ID
		}
		zipA, zipX := newFile("a.zip", nil), newFile("x.zip", nil)
		inA, inX := newFile("in a.jpg", &zipA), newFile("in x.jpg", &zipX)

		res, err := db.File().Query(ctx, models.FileQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: perPageAll()},
			FileFilter: &models.FileFilterType{ZipFile: &models.MultiCriterionInput{
				Excludes: []string{strconv.Itoa(int(zipX))}, Modifier: models.CriterionModifierIncludes,
			}},
		})
		require.NoError(t, err)
		assert.Contains(t, res.IDs, inA)
		assert.Contains(t, res.IDs, zipA, "a file that isn't in a zip file is kept")
		assert.NotContains(t, res.IDs, inX)
	})
}

// #2235: the folder zip file filter ignored the excludes list.
func TestFolderZipFileExcludesList(t *testing.T) {
	runWithRollbackTxn(t, "folders by zip file", func(t *testing.T, ctx context.Context) {
		root := &models.Folder{Path: filepath.Join(t.TempDir(), "folder zip excludes")}
		require.NoError(t, db.Folder().Create(ctx, root))
		newZip := func(name string) models.FileID {
			f := &models.BaseFile{ParentFolderID: root.ID, Basename: name}
			require.NoError(t, db.File().Create(ctx, f))
			return f.ID
		}
		newFolder := func(name string, zip *models.FileID) models.FolderID {
			f := &models.Folder{Path: filepath.Join(root.Path, name), ParentFolderID: &root.ID, DirEntry: models.DirEntry{ZipFileID: zip}}
			require.NoError(t, db.Folder().Create(ctx, f))
			return f.ID
		}
		zipA, zipX := newZip("a.zip"), newZip("x.zip")
		inA, inX, loose := newFolder("in a", &zipA), newFolder("in x", &zipX), newFolder("loose", nil)

		res, err := db.Folder().Query(ctx, models.FolderQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: perPageAll()},
			FolderFilter: &models.FolderFilterType{ZipFile: &models.MultiCriterionInput{
				Excludes: []string{strconv.Itoa(int(zipX))}, Modifier: models.CriterionModifierIncludes,
			}},
		})
		require.NoError(t, err)
		assert.Contains(t, res.IDs, inA)
		assert.Contains(t, res.IDs, loose, "a folder that isn't in a zip file is kept")
		assert.NotContains(t, res.IDs, inX)
	})
}

// #2235, #2197: an id too large for the id columns is no row's id, so it
// matches nothing, without an error, in every filter that takes ids.
// PostgreSQL id columns are 32-bit; SQLite must give the same results.
func TestFilterIDsOutOfRange(t *testing.T) {
	const big = "3000000000"
	all := -1
	scenes := func(ctx context.Context, f *models.SceneFilterType) ([]int, error) {
		res, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll()}, SceneFilter: f})
		if err != nil {
			return nil, err
		}
		return res.IDs, nil
	}
	performers := func(ctx context.Context, f *models.PerformerFilterType) ([]int, error) {
		ps, _, err := db.Performer().Query(ctx, f, perPageAll())
		var ids []int
		for _, p := range ps {
			ids = append(ids, p.ID)
		}
		return ids, err
	}

	done := false
	run := func(t *testing.T, ctx context.Context, c int) {
		tg := models.Tag{Name: "out of range tag"}
		require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: &tg}))
		var pids []int
		for _, n := range []string{"A", "B"} {
			p := models.Performer{Name: "out of range " + n}
			require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &p}))
			pids = append(pids, p.ID)
		}
		s := models.Scene{Title: "out of range scene", TagIDs: models.NewRelatedIDs([]int{tg.ID}), PerformerIDs: models.NewRelatedIDs(pids)}
		require.NoError(t, db.Scene().Create(ctx, &s, nil))
		tagID, pA, pB := strconv.Itoa(tg.ID), strconv.Itoa(pids[0]), strconv.Itoa(pids[1])

		// queries that must find nothing
		none := []struct {
			name  string
			query func() ([]int, error)
		}{
			{"scenes by tags, sub-tags", func() ([]int, error) {
				return scenes(ctx, &models.SceneFilterType{Tags: &models.HierarchicalMultiCriterionInput{Value: []string{big}, Modifier: models.CriterionModifierIncludes, Depth: &all}})
			}},
			{"scenes by tags, includes all", func() ([]int, error) {
				return scenes(ctx, &models.SceneFilterType{Tags: &models.HierarchicalMultiCriterionInput{Value: []string{tagID, big}, Modifier: models.CriterionModifierIncludesAll, Depth: &all}})
			}},
			{"scenes by galleries", func() ([]int, error) {
				return scenes(ctx, &models.SceneFilterType{Galleries: &models.MultiCriterionInput{Value: []string{big}, Modifier: models.CriterionModifierIncludes}})
			}},
			{"scenes by performers", func() ([]int, error) {
				return scenes(ctx, &models.SceneFilterType{Performers: &models.MultiCriterionInput{Value: []string{big}, Modifier: models.CriterionModifierIncludes}})
			}},
			{"scenes by performers, includes all", func() ([]int, error) {
				return scenes(ctx, &models.SceneFilterType{Performers: &models.MultiCriterionInput{Value: []string{pA, big}, Modifier: models.CriterionModifierIncludesAll}})
			}},
			{"scenes by performers, equals", func() ([]int, error) {
				return scenes(ctx, &models.SceneFilterType{Performers: &models.MultiCriterionInput{Value: []string{pA, pB, big}, Modifier: models.CriterionModifierEquals}})
			}},
			{"scenes by studios, sub-studios", func() ([]int, error) {
				return scenes(ctx, &models.SceneFilterType{Studios: &models.HierarchicalMultiCriterionInput{Value: []string{big}, Modifier: models.CriterionModifierIncludes, Depth: &all}})
			}},
			{"performers they appear with", func() ([]int, error) {
				return performers(ctx, &models.PerformerFilterType{Performers: &models.MultiCriterionInput{Value: []string{big}, Modifier: models.CriterionModifierIncludes}})
			}},
			{"performers by groups, sub-groups", func() ([]int, error) {
				return performers(ctx, &models.PerformerFilterType{Groups: &models.HierarchicalMultiCriterionInput{Value: []string{big}, Modifier: models.CriterionModifierIncludes, Depth: &all}})
			}},
			{"performers by studios, sub-studios", func() ([]int, error) {
				return performers(ctx, &models.PerformerFilterType{Studios: &models.HierarchicalMultiCriterionInput{Value: []string{big}, Modifier: models.CriterionModifierIncludes, Depth: &all}})
			}},
			{"tags by parent tags, all levels", func() ([]int, error) {
				ts, _, err := db.Tag().Query(ctx, &models.TagFilterType{Parents: &models.HierarchicalMultiCriterionInput{Value: []string{big}, Modifier: models.CriterionModifierIncludes, Depth: &all}}, perPageAll())
				var ids []int
				for _, x := range ts {
					ids = append(ids, x.ID)
				}
				return ids, err
			}},
			{"files by zip file", func() ([]int, error) {
				res, err := db.File().Query(ctx, models.FileQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll()}, FileFilter: &models.FileFilterType{ZipFile: &models.MultiCriterionInput{Value: []string{big}, Modifier: models.CriterionModifierIncludes}}})
				if err != nil {
					return nil, err
				}
				var ids []int
				for _, id := range res.IDs {
					ids = append(ids, int(id))
				}
				return ids, nil
			}},
			{"scene markers by tags, sub-tags", func() ([]int, error) {
				ms, _, err := db.SceneMarker().Query(ctx, &models.SceneMarkerFilterType{Tags: &models.HierarchicalMultiCriterionInput{Value: []string{big}, Modifier: models.CriterionModifierIncludes, Depth: &all}}, perPageAll())
				var ids []int
				for _, m := range ms {
					ids = append(ids, m.ID)
				}
				return ids, err
			}},
		}
		// an out-of-range id in an excludes list leaves nothing out
		keeps := []struct {
			name   string
			query  func() ([]int, error)
			wantID int
		}{
			{"scenes by tags, excludes list", func() ([]int, error) {
				return scenes(ctx, &models.SceneFilterType{Tags: &models.HierarchicalMultiCriterionInput{Value: []string{tagID}, Excludes: []string{big}, Modifier: models.CriterionModifierIncludes, Depth: &all}})
			}, s.ID},
			{"scenes by performers, excludes", func() ([]int, error) {
				return scenes(ctx, &models.SceneFilterType{Performers: &models.MultiCriterionInput{Value: []string{big}, Modifier: models.CriterionModifierExcludes}})
			}, s.ID},
			{"performers they appear with, excludes list", func() ([]int, error) {
				return performers(ctx, &models.PerformerFilterType{Performers: &models.MultiCriterionInput{Value: []string{pA}, Excludes: []string{big}, Modifier: models.CriterionModifierIncludes}})
			}, pids[1]},
		}
		// one case per transaction, as a failed query ends a PostgreSQL
		// transaction
		if c < len(none) {
			tt := none[c]
			ids, err := tt.query()
			if assert.NoError(t, err, tt.name) {
				assert.Empty(t, ids, tt.name)
			}
			return
		}
		if c-len(none) < len(keeps) {
			tt := keeps[c-len(none)]
			ids, err := tt.query()
			if assert.NoError(t, err, tt.name) {
				assert.Contains(t, ids, tt.wantID, tt.name)
			}
			return
		}
		done = true
	}

	for c := 0; !done; c++ {
		runWithRollbackTxn(t, "case "+strconv.Itoa(c), func(t *testing.T, ctx context.Context) { run(t, ctx, c) })
	}
}

// #2235: the zip file "excludes" modifier also dropped files and folders that
// aren't in any zip file. Like the Tags filter's excludes, which keeps scenes
// with no tags, it must keep them.
func TestZipFileExcludesKeepsUnzipped(t *testing.T) {
	runWithRollbackTxn(t, "files and folders", func(t *testing.T, ctx context.Context) {
		root := &models.Folder{Path: filepath.Join(t.TempDir(), "zip excludes modifier")}
		require.NoError(t, db.Folder().Create(ctx, root))
		newFile := func(name string, zip *models.FileID) models.FileID {
			f := &models.BaseFile{ParentFolderID: root.ID, Basename: name, DirEntry: models.DirEntry{ZipFileID: zip}}
			require.NoError(t, db.File().Create(ctx, f))
			return f.ID
		}
		zipA, zipX := newFile("a.zip", nil), newFile("x.zip", nil)
		inA, inX := newFile("in a.jpg", &zipA), newFile("in x.jpg", &zipX)
		excludeX := &models.MultiCriterionInput{Value: []string{strconv.Itoa(int(zipX))}, Modifier: models.CriterionModifierExcludes}

		res, err := db.File().Query(ctx, models.FileQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: perPageAll()},
			FileFilter:   &models.FileFilterType{ZipFile: excludeX},
		})
		require.NoError(t, err)
		assert.Contains(t, res.IDs, inA)
		assert.Contains(t, res.IDs, zipA, "a file that isn't in a zip file is kept")
		assert.NotContains(t, res.IDs, inX)

		// a folder inside zip X, and one that isn't in a zip file
		folderInX := &models.Folder{Path: filepath.Join(root.Path, "x.zip", "inside"), ParentFolderID: &root.ID, DirEntry: models.DirEntry{ZipFileID: &zipX}}
		require.NoError(t, db.Folder().Create(ctx, folderInX))
		loose := &models.Folder{Path: filepath.Join(root.Path, "loose"), ParentFolderID: &root.ID}
		require.NoError(t, db.Folder().Create(ctx, loose))

		folders, err := db.Folder().Query(ctx, models.FolderQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: perPageAll()},
			FolderFilter: &models.FolderFilterType{ZipFile: excludeX},
		})
		require.NoError(t, err)
		assert.Contains(t, folders.IDs, loose.ID, "a folder that isn't in a zip file is kept")
		assert.NotContains(t, folders.IDs, folderInX.ID)
	})
}
