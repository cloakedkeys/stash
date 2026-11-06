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
// a number must not change the query: it is refused, or matches nothing.
func TestFilterIDsNotInQueryText(t *testing.T) {
	// refusedOrEmpty passes when the query failed, or found nothing
	refusedOrEmpty := func(t *testing.T, n int, err error) {
		t.Helper()
		if err == nil {
			assert.Zero(t, n, "an id that isn't a number must not match anything")
		}
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
