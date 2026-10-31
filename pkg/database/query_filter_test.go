//go:build integration
// +build integration

package database_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// messages that mean the database rejected the query, rather than the filter
// being refused for having an unsuitable value
var sqlErrorMarkers = []string{"SQLSTATE", "SQL logic error", "no such column", "no such table", "syntax error", "ambiguous column"}

var filterModifiers = []models.CriterionModifier{
	models.CriterionModifierEquals,
	models.CriterionModifierNotEquals,
	models.CriterionModifierGreaterThan,
	models.CriterionModifierLessThan,
	models.CriterionModifierIsNull,
	models.CriterionModifierNotNull,
	models.CriterionModifierIncludes,
	models.CriterionModifierExcludes,
	models.CriterionModifierIncludesAll,
	models.CriterionModifierMatchesRegex,
	models.CriterionModifierNotMatchesRegex,
	models.CriterionModifierBetween,
	models.CriterionModifierNotBetween,
}

// sampleValue fills a value with something plausible for its type, field by
// field. The modifier is used for any Modifier field. Filters nest in each other
// without end, so a nested filter is only filled once.
func sampleValue(v reflect.Value, modifier models.CriterionModifier, fieldName string, inFilter bool) {
	switch v.Kind() {
	case reflect.Ptr:
		isFilter := strings.HasSuffix(v.Type().Elem().Name(), "FilterType")
		if isFilter && inFilter {
			return
		}
		v.Set(reflect.New(v.Type().Elem()))
		sampleValue(v.Elem(), modifier, fieldName, inFilter || isFilter)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() || f.Name == "OperatorFilter" {
				continue
			}
			sampleValue(v.Field(i), modifier, f.Name, inFilter)
		}
	case reflect.Slice:
		// two values, since some modifiers need that many
		s := reflect.MakeSlice(v.Type(), 2, 2)
		for i := 0; i < s.Len(); i++ {
			if s.Index(i).Kind() == reflect.String {
				// lists of strings are almost always IDs
				s.Index(i).SetString("1")
			} else if s.Index(i).Kind() == reflect.Interface {
				s.Index(i).Set(reflect.ValueOf("1"))
			} else {
				sampleValue(s.Index(i), modifier, fieldName, inFilter)
			}
		}
		v.Set(s)
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
	case reflect.String:
		switch {
		case v.Type() == reflect.TypeOf(modifier):
			v.SetString(string(modifier))
		case strings.HasSuffix(fieldName, "ID"):
			v.SetString("1")
		default:
			// a date works for both text and date columns, and is valid as a regex
			v.SetString("2020-01-01")
		}
	case reflect.Int, reflect.Int64:
		v.SetInt(1)
	case reflect.Float64:
		v.SetFloat(1)
	case reflect.Bool:
		v.SetBool(true)
	}
}

// filterVariants returns one filter for each settable criterion and modifier.
func filterVariants[T any]() map[string]*T {
	ret := make(map[string]*T)
	typ := reflect.TypeOf((*T)(nil)).Elem()
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() || f.Name == "OperatorFilter" {
			continue
		}
		for _, modifier := range filterModifiers {
			filter := new(T)
			sampleValue(reflect.ValueOf(filter).Elem().Field(i), modifier, f.Name, false)
			ret[f.Name+" "+string(modifier)] = filter
		}
	}
	return ret
}

func runFilterVariants[T any](t *testing.T, name string, query func(ctx context.Context, filter *T) (int, error)) {
	for variant, filter := range filterVariants[T]() {
		runWithRollbackTxn(t, name+" "+variant, func(t *testing.T, ctx context.Context) {
			// the where clause helpers panic on a modifier that doesn't suit the
			// value type, which is a refused filter and not what's being tested
			defer func() {
				if r := recover(); r != nil && !strings.Contains(fmt.Sprint(r), "modifier") {
					panic(r)
				}
			}()

			count, err := query(ctx, filter)
			if err == nil {
				// logged so runs against different backends can be compared
				t.Logf("matched %d", count)
				return
			}
			for _, marker := range sqlErrorMarkers {
				assert.NotContains(t, err.Error(), marker)
			}
		})
	}
}

// Every filter, with every modifier, must give a query the database accepts on
// every backend. Results aren't checked here. Nested filters such as
// performers_filter are included, with all of their criteria set at once.
func TestQueryAllFilters(t *testing.T) {
	findFilter := &models.FindFilterType{}
	count := models.QueryOptions{FindFilter: findFilter, Count: true}

	runFilterVariants(t, "scene", func(ctx context.Context, f *models.SceneFilterType) (int, error) {
		r, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: count, SceneFilter: f, TotalDuration: true, TotalSize: true})
		if err != nil {
			return 0, err
		}
		return r.Count, nil
	})
	runFilterVariants(t, "image", func(ctx context.Context, f *models.ImageFilterType) (int, error) {
		r, err := db.Image().Query(ctx, models.ImageQueryOptions{QueryOptions: count, ImageFilter: f, Megapixels: true, TotalSize: true})
		if err != nil {
			return 0, err
		}
		return r.Count, nil
	})
	runFilterVariants(t, "audio", func(ctx context.Context, f *models.AudioFilterType) (int, error) {
		r, err := db.Audio().Query(ctx, models.AudioQueryOptions{QueryOptions: count, AudioFilter: f, TotalDuration: true, TotalSize: true})
		if err != nil {
			return 0, err
		}
		return r.Count, nil
	})
	runFilterVariants(t, "file", func(ctx context.Context, f *models.FileFilterType) (int, error) {
		r, err := db.File().Query(ctx, models.FileQueryOptions{QueryOptions: count, FileFilter: f, TotalDuration: true, Megapixels: true, TotalSize: true})
		if err != nil {
			return 0, err
		}
		return r.Count, nil
	})
	runFilterVariants(t, "folder", func(ctx context.Context, f *models.FolderFilterType) (int, error) {
		r, err := db.Folder().Query(ctx, models.FolderQueryOptions{QueryOptions: count, FolderFilter: f})
		if err != nil {
			return 0, err
		}
		return r.Count, nil
	})
	runFilterVariants(t, "gallery", func(ctx context.Context, f *models.GalleryFilterType) (int, error) {
		_, count, err := db.Gallery().Query(ctx, f, findFilter)
		return count, err
	})
	runFilterVariants(t, "group", func(ctx context.Context, f *models.GroupFilterType) (int, error) {
		_, count, err := db.Group().Query(ctx, f, findFilter)
		return count, err
	})
	runFilterVariants(t, "performer", func(ctx context.Context, f *models.PerformerFilterType) (int, error) {
		_, count, err := db.Performer().Query(ctx, f, findFilter)
		return count, err
	})
	runFilterVariants(t, "scene marker", func(ctx context.Context, f *models.SceneMarkerFilterType) (int, error) {
		_, count, err := db.SceneMarker().Query(ctx, f, findFilter)
		return count, err
	})
	runFilterVariants(t, "studio", func(ctx context.Context, f *models.StudioFilterType) (int, error) {
		_, count, err := db.Studio().Query(ctx, f, findFilter)
		return count, err
	})
	runFilterVariants(t, "tag", func(ctx context.Context, f *models.TagFilterType) (int, error) {
		_, count, err := db.Tag().Query(ctx, f, findFilter)
		return count, err
	})
}

// A markers filter must only match scenes that have a matching marker.
func TestSceneQueryMarkersFilter(t *testing.T) {
	runWithRollbackTxn(t, "scene markers filter", func(t *testing.T, ctx context.Context) {
		perPage := -1
		findFilter := &models.FindFilterType{PerPage: &perPage}

		markers, _, err := db.SceneMarker().Query(ctx, nil, findFilter)
		require.NoError(t, err)
		require.NotEmpty(t, markers)

		sceneIDs := make(map[int]bool)
		for _, marker := range markers {
			sceneIDs[marker.SceneID] = true
		}

		result, err := db.Scene().Query(ctx, models.SceneQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: findFilter},
			SceneFilter: &models.SceneFilterType{
				MarkersFilter: &models.SceneMarkerFilterType{
					CreatedAt: &models.TimestampCriterionInput{
						Value:    "3000-01-01 00:00",
						Modifier: models.CriterionModifierLessThan,
					},
				},
			},
		})
		require.NoError(t, err)

		got := make(map[int]bool)
		for _, id := range result.IDs {
			got[id] = true
		}
		assert.Equal(t, sceneIDs, got)
	})
}

// A folders filter must only match galleries in a matching folder, whichever
// folder criterion is used.
func TestGalleryQueryFoldersFilter(t *testing.T) {
	runWithRollbackTxn(t, "gallery folders filter", func(t *testing.T, ctx context.Context) {
		folderID := folderIDs[folderIdxWithGalleryFiles]
		otherFolderID := folderIDs[folderIdxForObjectFiles]

		inFolder := models.Gallery{Title: "in folder", FolderID: &folderID}
		require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: &inFolder}))
		otherFolder := models.Gallery{Title: "other folder", FolderID: &otherFolderID}
		require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: &otherFolder}))

		folder, err := db.Folder().Find(ctx, folderID)
		require.NoError(t, err)

		tests := map[string]*models.FolderFilterType{
			"path": {
				Path: &models.StringCriterionInput{Value: folder.Path, Modifier: models.CriterionModifierEquals},
			},
			"gallery count": {
				Path:         &models.StringCriterionInput{Value: folder.Path, Modifier: models.CriterionModifierEquals},
				GalleryCount: &models.IntCriterionInput{Value: 0, Modifier: models.CriterionModifierGreaterThan},
			},
		}

		for name, filter := range tests {
			galleries, _, err := db.Gallery().Query(ctx, &models.GalleryFilterType{FoldersFilter: filter}, nil)
			require.NoError(t, err, name)

			ids := make([]int, len(galleries))
			for i, g := range galleries {
				ids[i] = g.ID
			}
			assert.Contains(t, ids, inFolder.ID, name)
			assert.NotContains(t, ids, otherFolder.ID, name)
		}
	})
}
