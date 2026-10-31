//go:build integration
// +build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
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
