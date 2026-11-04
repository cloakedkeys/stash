//go:build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findManyFunc runs a store's FindMany and returns the id and the object found
// at each position.
type findManyFunc func(ctx context.Context, ids []int) ([]int, []any, error)

func findManyOf[T any](find func(context.Context, []int) ([]*T, error), id func(*T) int) findManyFunc {
	return func(ctx context.Context, ids []int) ([]int, []any, error) {
		found, err := find(ctx, ids)
		if err != nil {
			return nil, nil, err
		}

		gotIDs := make([]int, len(found))
		objs := make([]any, len(found))
		for i, f := range found {
			if f == nil {
				continue
			}
			gotIDs[i] = id(f)
			objs[i] = f
		}
		return gotIDs, objs, nil
	}
}

type userFindMany interface {
	FindMany(ctx context.Context, ids []int) ([]*models.User, error)
}

// TestFindManyRepeatedIDs checks that an id listed more than once finds the
// same object at each of its positions, and that an id that does not exist
// still fails (#2078).
func TestFindManyRepeatedIDs(t *testing.T) {
	createStory := func(t *testing.T, ctx context.Context, title string) int {
		t.Helper()
		s := models.NewStory()
		s.Title = title
		require.NoError(t, db.Story().Create(ctx, &s, nil))
		return s.ID
	}

	createUser := func(t *testing.T, ctx context.Context) int {
		t.Helper()
		u := models.User{Username: "find-many-repeated", Roles: models.Roles{models.RoleEnumRead}}
		require.NoError(t, db.User().Create(ctx, &u, "password-hash"))
		return u.ID
	}

	tests := []struct {
		name string
		// ids returns distinct ids that exist
		ids  func(t *testing.T, ctx context.Context) []int
		find findManyFunc
	}{
		{
			"scene",
			func(*testing.T, context.Context) []int { return []int{sceneIDs[0], sceneIDs[1]} },
			findManyOf(db.Scene().FindMany, func(v *models.Scene) int { return v.ID }),
		},
		{
			"image",
			func(*testing.T, context.Context) []int { return []int{imageIDs[0], imageIDs[1]} },
			findManyOf(db.Image().FindMany, func(v *models.Image) int { return v.ID }),
		},
		{
			"gallery",
			func(*testing.T, context.Context) []int { return []int{galleryIDs[0], galleryIDs[1]} },
			findManyOf(db.Gallery().FindMany, func(v *models.Gallery) int { return v.ID }),
		},
		{
			"gallery chapter",
			func(*testing.T, context.Context) []int { return []int{chapterIDs[0]} },
			findManyOf(db.GalleryChapter().FindMany, func(v *models.GalleryChapter) int { return v.ID }),
		},
		{
			"performer",
			func(*testing.T, context.Context) []int { return []int{performerIDs[0], performerIDs[1]} },
			findManyOf(db.Performer().FindMany, func(v *models.Performer) int { return v.ID }),
		},
		{
			"studio",
			func(*testing.T, context.Context) []int { return []int{studioIDs[0], studioIDs[1]} },
			findManyOf(db.Studio().FindMany, func(v *models.Studio) int { return v.ID }),
		},
		{
			"tag",
			func(*testing.T, context.Context) []int { return []int{tagIDs[0], tagIDs[1]} },
			findManyOf(db.Tag().FindMany, func(v *models.Tag) int { return v.ID }),
		},
		{
			"group",
			func(*testing.T, context.Context) []int { return []int{groupIDs[0], groupIDs[1]} },
			findManyOf(db.Group().FindMany, func(v *models.Group) int { return v.ID }),
		},
		{
			"audio",
			func(*testing.T, context.Context) []int { return []int{audioIDs[0], audioIDs[1]} },
			findManyOf(db.Audio().FindMany, func(v *models.Audio) int { return v.ID }),
		},
		{
			"story",
			func(t *testing.T, ctx context.Context) []int {
				return []int{createStory(t, ctx, "repeated one"), createStory(t, ctx, "repeated two")}
			},
			findManyOf(db.Story().FindMany, func(v *models.Story) int { return v.ID }),
		},
		{
			"scene marker",
			func(*testing.T, context.Context) []int { return []int{markerIDs[0], markerIDs[1]} },
			findManyOf(db.SceneMarker().FindMany, func(v *models.SceneMarker) int { return v.ID }),
		},
		{
			"saved filter",
			func(*testing.T, context.Context) []int { return []int{savedFilterIDs[0], savedFilterIDs[1]} },
			findManyOf(func(ctx context.Context, ids []int) ([]*models.SavedFilter, error) {
				return db.SavedFilter().FindMany(ctx, ids, false)
			}, func(v *models.SavedFilter) int { return v.ID }),
		},
		{
			"folder",
			func(*testing.T, context.Context) []int { return []int{int(folderIDs[0]), int(folderIDs[1])} },
			findManyOf(func(ctx context.Context, ids []int) ([]*models.Folder, error) {
				folderIDs := make([]models.FolderID, len(ids))
				for i, id := range ids {
					folderIDs[i] = models.FolderID(id)
				}
				return db.Folder().FindMany(ctx, folderIDs)
			}, func(v *models.Folder) int { return int(v.ID) }),
		},
		{
			"user",
			func(t *testing.T, ctx context.Context) []int { return []int{testUserID, createUser(t, ctx)} },
			findManyOf(func(ctx context.Context, ids []int) ([]*models.User, error) {
				store, ok := db.User().(userFindMany)
				if !ok {
					t.Fatal("user store has no FindMany")
				}
				return store.FindMany(ctx, ids)
			}, func(v *models.User) int { return v.ID }),
		},
	}

	for _, tt := range tests {
		runWithRollbackTxn(t, tt.name+" repeated", func(t *testing.T, ctx context.Context) {
			distinct := tt.ids(t, ctx)
			first := distinct[0]

			// the first id at the start, in the middle and at the end
			ids := append([]int{first, first}, distinct...)
			ids = append(ids, first)

			gotIDs, objs, err := tt.find(ctx, ids)
			require.NoError(t, err)
			assert.Equal(t, ids, gotIDs)
			for i, id := range ids {
				if id == first {
					assert.Same(t, objs[0], objs[i], "position %d", i)
				}
			}
		})

		runWithRollbackTxn(t, tt.name+" repeated and missing", func(t *testing.T, ctx context.Context) {
			first := tt.ids(t, ctx)[0]
			_, _, err := tt.find(ctx, []int{first, first, invalidID})
			assert.Error(t, err)
		})
	}
}

// TestFindManyRepeatedIDsIgnoreNotFound checks that a saved filter lookup
// that ignores missing ids still fills every position of a repeated id.
func TestFindManyRepeatedIDsIgnoreNotFound(t *testing.T) {
	runWithRollbackTxn(t, "saved filter", func(t *testing.T, ctx context.Context) {
		id := savedFilterIDs[0]
		got, err := db.SavedFilter().FindMany(ctx, []int{id, invalidID, id}, true)
		require.NoError(t, err)
		require.Len(t, got, 3)
		require.NotNil(t, got[0])
		assert.Nil(t, got[1])
		assert.Same(t, got[0], got[2])
	})
}

// TestBulkLookupRepeatedIDs checks the per-id bulk lookups used by the data
// loaders: an id listed twice gets the same result at both positions.
func TestBulkLookupRepeatedIDs(t *testing.T) {
	sceneID := sceneIDs[0]
	otherSceneID := sceneIDs[1]
	ids := []int{sceneID, otherSceneID, sceneID}
	viewed := time.Date(2021, 3, 4, 5, 6, 7, 0, time.UTC)

	runWithRollbackTxn(t, "scene file ids", func(t *testing.T, ctx context.Context) {
		got, err := db.Scene().GetManyFileIDs(ctx, ids)
		require.NoError(t, err)
		require.Len(t, got, 3)
		assert.NotEmpty(t, got[0])
		assert.Equal(t, got[0], got[2])
	})

	runWithRollbackTxn(t, "scene views", func(t *testing.T, ctx context.Context) {
		qb := db.Scene()
		_, err := qb.AddViews(ctx, sceneID, []time.Time{viewed})
		require.NoError(t, err)

		dates, err := qb.GetManyViewDates(ctx, ids)
		require.NoError(t, err)
		require.Len(t, dates, 3)
		assert.NotEmpty(t, dates[0])
		assert.Equal(t, dates[0], dates[2])

		counts, err := qb.GetManyViewCount(ctx, ids)
		require.NoError(t, err)
		require.Len(t, counts, 3)
		assert.NotZero(t, counts[0])
		assert.Equal(t, counts[0], counts[2])

		last, err := qb.GetManyLastViewed(ctx, ids)
		require.NoError(t, err)
		require.Len(t, last, 3)
		require.NotNil(t, last[0])
		require.NotNil(t, last[2])
		assert.True(t, last[0].Equal(*last[2]))
	})

	runWithRollbackTxn(t, "scene o dates", func(t *testing.T, ctx context.Context) {
		qb := db.Scene()
		_, err := qb.AddO(ctx, sceneID, []time.Time{viewed})
		require.NoError(t, err)

		dates, err := qb.GetManyODates(ctx, ids)
		require.NoError(t, err)
		require.Len(t, dates, 3)
		assert.NotEmpty(t, dates[0])
		assert.Equal(t, dates[0], dates[2])
	})

	runWithRollbackTxn(t, "performer custom fields", func(t *testing.T, ctx context.Context) {
		qb := db.Performer()
		id := performerIDs[0]
		require.NoError(t, qb.SetCustomFields(ctx, id, models.CustomFieldsInput{
			Full: map[string]interface{}{"repeated": "value"},
		}))

		got, err := qb.GetCustomFieldsBulk(ctx, []int{id, performerIDs[1], id})
		require.NoError(t, err)
		require.Len(t, got, 3)
		assert.Equal(t, "value", got[0]["repeated"])
		assert.Equal(t, got[0], got[2])
	})
}
