//go:build integration
// +build integration

package database_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every sort option, with and without the totals shown in list views, must give
// a valid query on every database backend. The list of options is written out
// here so that adding one to only one backend fails this test.
func TestQueryAllSortOptions(t *testing.T) {
	find := func(sort string, direction models.SortDirectionEnum) models.QueryOptions {
		return models.QueryOptions{
			FindFilter: &models.FindFilterType{Sort: &sort, Direction: &direction},
			Count:      true,
		}
	}

	tests := []struct {
		name  string
		sorts []string
		query func(ctx context.Context, sort string, direction models.SortDirectionEnum) error
	}{
		{
			name:  "scene",
			sorts: []string{"bitrate", "created_at", "code", "date", "production_date", "file_count", "filesize", "duration", "file_mod_time", "framerate", "group_scene_number", "id", "interactive", "interactive_speed", "last_o_at", "last_played_at", "movie_scene_number", "o_counter", "organized", "performer_count", "play_count", "play_duration", "resume_time", "path", "perceptual_similarity", "random", "rating", "resolution", "studio", "tag_count", "title", "updated_at", "performer_age"},
			query: func(ctx context.Context, sort string, direction models.SortDirectionEnum) error {
				for _, totals := range []bool{false, true} {
					if _, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: find(sort, direction), TotalDuration: totals, TotalSize: totals}); err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			name:  "image",
			sorts: []string{"created_at", "date", "file_count", "file_mod_time", "filesize", "id", "o_counter", "path", "performer_age", "performer_count", "random", "rating", "resolution", "tag_count", "title", "updated_at"},
			query: func(ctx context.Context, sort string, direction models.SortDirectionEnum) error {
				for _, totals := range []bool{false, true} {
					if _, err := db.Image().Query(ctx, models.ImageQueryOptions{QueryOptions: find(sort, direction), Megapixels: totals, TotalSize: totals}); err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			name:  "audio",
			sorts: []string{"bitrate", "created_at", "code", "date", "file_count", "filesize", "duration", "file_mod_time", "sample_rate", "group_audio_number", "id", "last_o_at", "last_played_at", "o_counter", "organized", "performer_count", "play_count", "play_duration", "resume_time", "path", "random", "rating", "studio", "tag_count", "title", "updated_at", "performer_age"},
			query: func(ctx context.Context, sort string, direction models.SortDirectionEnum) error {
				for _, totals := range []bool{false, true} {
					if _, err := db.Audio().Query(ctx, models.AudioQueryOptions{QueryOptions: find(sort, direction), TotalDuration: totals, TotalSize: totals}); err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			name:  "file",
			sorts: []string{"created_at", "id", "path", "random", "missing_since", "updated_at"},
			query: func(ctx context.Context, sort string, direction models.SortDirectionEnum) error {
				for _, totals := range []bool{false, true} {
					if _, err := db.File().Query(ctx, models.FileQueryOptions{QueryOptions: find(sort, direction), TotalDuration: totals, Megapixels: totals, TotalSize: totals}); err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			name:  "folder",
			sorts: []string{"created_at", "id", "path", "basename", "random", "missing_since", "updated_at"},
			query: func(ctx context.Context, sort string, direction models.SortDirectionEnum) error {
				_, err := db.Folder().Query(ctx, models.FolderQueryOptions{QueryOptions: find(sort, direction)})
				return err
			},
		},
		{
			name:  "gallery",
			sorts: []string{"created_at", "date", "file_count", "file_mod_time", "id", "images_count", "o_counter", "path", "performer_age", "performer_count", "random", "rating", "tag_count", "title", "updated_at"},
			query: func(ctx context.Context, sort string, direction models.SortDirectionEnum) error {
				_, _, err := db.Gallery().Query(ctx, nil, find(sort, direction).FindFilter)
				return err
			},
		},
		{
			name:  "group",
			sorts: []string{"created_at", "date", "duration", "id", "name", "random", "rating", "scenes_count", "audios_count", "o_counter", "performer_count", "tag_count", "updated_at"},
			query: func(ctx context.Context, sort string, direction models.SortDirectionEnum) error {
				_, _, err := db.Group().Query(ctx, nil, find(sort, direction).FindFilter)
				return err
			},
		},
		{
			name:  "performer",
			sorts: []string{"birthdate", "career_start", "career_end", "created_at", "galleries_count", "height", "id", "images_count", "last_o_at", "last_played_at", "latest_scene", "latest_audio", "measurements", "name", "o_counter", "penis_length", "play_count", "random", "rating", "favorite", "scenes_count", "scenes_duration", "scenes_size", "audios_count", "audios_duration", "audios_size", "tag_count", "updated_at", "weight"},
			query: func(ctx context.Context, sort string, direction models.SortDirectionEnum) error {
				_, _, err := db.Performer().Query(ctx, nil, find(sort, direction).FindFilter)
				return err
			},
		},
		{
			name:  "scene marker",
			sorts: []string{"created_at", "id", "title", "random", "scene_id", "scenes_updated_at", "seconds", "updated_at", "duration"},
			query: func(ctx context.Context, sort string, direction models.SortDirectionEnum) error {
				_, _, err := db.SceneMarker().Query(ctx, nil, find(sort, direction).FindFilter)
				return err
			},
		},
		{
			name:  "studio",
			sorts: []string{"child_count", "created_at", "galleries_count", "id", "images_count", "latest_scene", "name", "favorite", "o_counter", "o_counter_all", "performer_count", "performer_count_all", "scenes_count", "scenes_count_all", "scenes_duration", "scenes_size", "scene_markers_count", "audios_count", "audios_duration", "audios_size", "random", "rating", "tag_count", "updated_at"},
			query: func(ctx context.Context, sort string, direction models.SortDirectionEnum) error {
				_, _, err := db.Studio().Query(ctx, nil, find(sort, direction).FindFilter)
				return err
			},
		},
		{
			name:  "tag",
			sorts: []string{"created_at", "favorite", "galleries_count", "groups_count", "id", "images_count", "movies_count", "rating", "studios_count", "name", "performers_count", "random", "scene_markers_count", "scenes_count", "scenes_duration", "scenes_size", "audios_count", "audios_duration", "audios_size", "updated_at"},
			query: func(ctx context.Context, sort string, direction models.SortDirectionEnum) error {
				_, _, err := db.Tag().Query(ctx, nil, find(sort, direction).FindFilter)
				return err
			},
		},
	}

	for _, tt := range tests {
		for _, sort := range tt.sorts {
			for _, direction := range []models.SortDirectionEnum{models.SortDirectionEnumAsc, models.SortDirectionEnumDesc} {
				runWithRollbackTxn(t, tt.name+" "+sort+" "+direction.String(), func(t *testing.T, ctx context.Context) {
					assert.NoError(t, tt.query(ctx, sort, direction))
				})
			}
		}
	}
}

// An item with several files must be listed once under every sort that looks at
// its files, with and without a filter that joins them. Listing it twice makes
// the whole query fail, since the items are then looked up by ID.
func TestQueryAllSortOptionsSeveralFiles(t *testing.T) {
	checkIDs := func(t *testing.T, ids []int) {
		seen := make(map[int]bool)
		for _, id := range ids {
			assert.False(t, seen[id], "id %d listed more than once", id)
			seen[id] = true
		}
	}

	pathFilter := &models.StringCriterionInput{Value: "e", Modifier: models.CriterionModifierIncludes}
	perPage := -1
	find := func(sort string) *models.FindFilterType {
		return &models.FindFilterType{Sort: &sort, PerPage: &perPage}
	}

	runSorts := func(name string, sorts []string, setup func(ctx context.Context) error, query func(ctx context.Context, t *testing.T, sort string, filtered bool) []int) {
		for _, sort := range sorts {
			for _, filtered := range []bool{false, true} {
				runWithRollbackTxn(t, fmt.Sprintf("%s %s filtered %v", name, sort, filtered), func(t *testing.T, ctx context.Context) {
					require.NoError(t, setup(ctx))
					checkIDs(t, query(ctx, t, sort, filtered))
				})
			}
		}
	}

	runSorts("scene", []string{"path", "title", "bitrate", "duration", "framerate", "file_mod_time", "resolution", "filesize", "perceptual_similarity"},
		func(ctx context.Context) error {
			return db.Scene().AssignFiles(ctx, sceneIDs[sceneIdxWithGroup], []models.FileID{sceneFileIDs[sceneIdxWithGallery]})
		},
		func(ctx context.Context, t *testing.T, sort string, filtered bool) []int {
			f := &models.SceneFilterType{}
			if filtered {
				f.Path = pathFilter
			}
			r, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: models.QueryOptions{FindFilter: find(sort), Count: true}, SceneFilter: f})
			require.NoError(t, err)
			_, err = r.Resolve(ctx)
			require.NoError(t, err)
			return r.IDs
		})

	runSorts("audio", []string{"path", "title", "bitrate", "duration", "sample_rate", "file_mod_time", "filesize"},
		func(ctx context.Context) error {
			return db.Audio().AssignFiles(ctx, audioIDs[audioIdxWithGroup], []models.FileID{audioFileIDs[audioIdxWithGallery]})
		},
		func(ctx context.Context, t *testing.T, sort string, filtered bool) []int {
			f := &models.AudioFilterType{}
			if filtered {
				f.Path = pathFilter
			}
			r, err := db.Audio().Query(ctx, models.AudioQueryOptions{QueryOptions: models.QueryOptions{FindFilter: find(sort), Count: true}, AudioFilter: f})
			require.NoError(t, err)
			_, err = r.Resolve(ctx)
			require.NoError(t, err)
			return r.IDs
		})

	runSorts("image", []string{"path", "title", "file_mod_time", "resolution", "filesize"},
		func(ctx context.Context) error {
			return db.Image().AddFileID(ctx, imageIDs[imageIdxWithGallery], imageFileIDs[imageIdx1WithGallery])
		},
		func(ctx context.Context, t *testing.T, sort string, filtered bool) []int {
			f := &models.ImageFilterType{}
			if filtered {
				f.Path = pathFilter
			}
			r, err := db.Image().Query(ctx, models.ImageQueryOptions{QueryOptions: models.QueryOptions{FindFilter: find(sort), Count: true}, ImageFilter: f})
			require.NoError(t, err)
			_, err = r.Resolve(ctx)
			require.NoError(t, err)
			return r.IDs
		})

	runSorts("gallery", []string{"path", "title", "file_mod_time"},
		func(ctx context.Context) error {
			// not every gallery has a file
			var withFiles []int
			for i, id := range galleryFileIDs {
				if id != 0 {
					withFiles = append(withFiles, i)
				}
			}
			return db.Gallery().AddFileID(ctx, galleryIDs[withFiles[0]], galleryFileIDs[withFiles[1]])
		},
		func(ctx context.Context, t *testing.T, sort string, filtered bool) []int {
			f := &models.GalleryFilterType{}
			if filtered {
				f.Path = pathFilter
			}
			galleries, _, err := db.Gallery().Query(ctx, f, find(sort))
			require.NoError(t, err)
			ids := make([]int, len(galleries))
			for i, g := range galleries {
				ids[i] = g.ID
			}
			return ids
		})
}

// With an item that has several files, paging through a list sorted by a file's
// details must give every item once, the count must match, and the totals must
// not depend on the sort.
func TestQueryPagesSeveralFiles(t *testing.T) {
	pathFilter := &models.StringCriterionInput{Value: "e", Modifier: models.CriterionModifierIncludes}

	sceneQuery := func(ctx context.Context, t *testing.T, sort string, page, perPage int, filtered bool) *models.SceneQueryResult {
		f := &models.SceneFilterType{}
		if filtered {
			f.Path = pathFilter
		}
		r, err := db.Scene().Query(ctx, models.SceneQueryOptions{
			QueryOptions:  models.QueryOptions{FindFilter: &models.FindFilterType{Sort: &sort, Page: &page, PerPage: &perPage}, Count: true},
			SceneFilter:   f,
			TotalDuration: true,
			TotalSize:     true,
		})
		require.NoError(t, err)
		return r
	}

	for _, sort := range []string{"path", "title", "duration", "filesize", "date"} {
		for _, filtered := range []bool{false, true} {
			runWithRollbackTxn(t, fmt.Sprintf("scene %s filtered %v", sort, filtered), func(t *testing.T, ctx context.Context) {
				require.NoError(t, db.Scene().AssignFiles(ctx, sceneIDs[sceneIdxWithGroup], []models.FileID{sceneFileIDs[sceneIdxWithGallery]}))

				all := sceneQuery(ctx, t, sort, 1, -1, filtered)
				assert.Equal(t, len(all.IDs), all.Count, "count")

				var paged []int
				for page := 1; page <= all.Count; page++ {
					r := sceneQuery(ctx, t, sort, page, 1, filtered)
					assert.Len(t, r.IDs, 1, "page %d", page)
					paged = append(paged, r.IDs...)
				}
				assert.Equal(t, all.IDs, paged, "paging gives the same list")

				// the totals of a sort that doesn't look at files
				byDate := sceneQuery(ctx, t, "date", 1, -1, filtered)
				assert.Equal(t, byDate.TotalSize, all.TotalSize, "total size")
				assert.Equal(t, byDate.TotalDuration, all.TotalDuration, "total duration")
			})
		}
	}

	// galleries page in a different query
	runWithRollbackTxn(t, "gallery path", func(t *testing.T, ctx context.Context) {
		var withFiles []int
		for i, id := range galleryFileIDs {
			if id != 0 {
				withFiles = append(withFiles, i)
			}
		}
		require.NoError(t, db.Gallery().AddFileID(ctx, galleryIDs[withFiles[0]], galleryFileIDs[withFiles[1]]))

		query := func(page, perPage int) ([]int, int) {
			sort := "path"
			galleries, count, err := db.Gallery().Query(ctx, &models.GalleryFilterType{Path: pathFilter}, &models.FindFilterType{Sort: &sort, Page: &page, PerPage: &perPage})
			require.NoError(t, err)
			ids := make([]int, len(galleries))
			for i, g := range galleries {
				ids[i] = g.ID
			}
			return ids, count
		}

		all, count := query(1, -1)
		assert.Equal(t, len(all), count, "count")
		var paged []int
		for page := 1; page <= count; page++ {
			ids, _ := query(page, 1)
			assert.Len(t, ids, 1, "page %d", page)
			paged = append(paged, ids...)
		}
		assert.Equal(t, all, paged, "paging gives the same list")
	})
}
