//go:build integration

package database_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stashapp/stash/pkg/group"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSceneFavorite(t *testing.T) {
	require.NoError(t, withRollbackTxn(func(ctx context.Context) error {
		qb := db.Scene()
		other := models.User{Username: "scene-favorites", Roles: models.Roles{models.RoleEnumModify}}
		require.NoError(t, db.User().Create(ctx, &other, ""))
		otherCtx := models.WithUserID(ctx, other.ID)
		guestCtx := models.WithUserID(ctx, 0)
		create := func(c context.Context, favorite bool) int {
			item := models.NewScene()
			item.Favorite = favorite
			item.Title = "favorite test"
			require.NoError(t, qb.Create(c, &item, nil))
			return item.ID
		}
		first := create(ctx, true)
		second := create(ctx, false)
		absent := create(guestCtx, false)
		// Full create, full update and partial update all use the current user's row.
		item, err := qb.Find(ctx, first)
		require.NoError(t, err)
		assert.True(t, item.Favorite)
		item.Favorite = false
		require.NoError(t, qb.Update(ctx, item))
		item, err = qb.Find(ctx, first)
		require.NoError(t, err)
		assert.False(t, item.Favorite)
		_, err = qb.UpdatePartial(ctx, first, models.ScenePartial{Favorite: models.NewOptionalBool(true)})
		require.NoError(t, err)
		_, err = qb.UpdatePartial(otherCtx, second, models.ScenePartial{Favorite: models.NewOptionalBool(true)})
		require.NoError(t, err)
		// Updating the rating leaves an existing favorite intact.
		_, err = qb.UpdatePartial(ctx, first, models.ScenePartial{Rating: models.NewOptionalInt(60)})
		require.NoError(t, err)
		for _, c := range []context.Context{ctx, otherCtx, guestCtx} {
			for _, id := range []int{first, second, absent} {
				item, err := qb.Find(c, id)
				require.NoError(t, err)
				assert.Equal(t, c == ctx && id == first || c == otherCtx && id == second, item.Favorite)
			}
		}
		query := func(c context.Context, favorite *bool, direction models.SortDirectionEnum) []int {
			sort := "favorite"
			perPage := -1
			find := &models.FindFilterType{Sort: &sort, Direction: &direction, PerPage: &perPage}
			filter := &models.SceneFilterType{ID: &models.IntCriterionInput{Value: first, Value2: &absent, Modifier: models.CriterionModifierBetween}, Favorite: favorite}
			var ids []int
			for _, item := range queryScene(c, t, qb, filter, find) {
				ids = append(ids, item.ID)
			}
			return ids
		}
		yes, no := true, false
		for _, tc := range []struct {
			c        context.Context
			favorite int
			others   []int
		}{
			{ctx, first, []int{second, absent}},
			{otherCtx, second, []int{first, absent}},
		} {
			assert.Equal(t, []int{tc.favorite}, query(tc.c, &yes, models.SortDirectionEnumAsc))
			assert.ElementsMatch(t, tc.others, query(tc.c, &no, models.SortDirectionEnumAsc))
			asc := query(tc.c, nil, models.SortDirectionEnumAsc)
			desc := query(tc.c, nil, models.SortDirectionEnumDesc)
			require.Len(t, asc, 3)
			require.Len(t, desc, 3)
			assert.Equal(t, tc.favorite, asc[2])
			assert.Equal(t, tc.favorite, desc[0])
		}
		assert.Empty(t, query(guestCtx, &yes, models.SortDirectionEnumAsc))
		assert.ElementsMatch(t, []int{first, second, absent}, query(guestCtx, &no, models.SortDirectionEnumAsc))
		return nil
	}))
}

func TestImageFavorite(t *testing.T) {
	require.NoError(t, withRollbackTxn(func(ctx context.Context) error {
		qb := db.Image()
		other := models.User{Username: "image-favorites", Roles: models.Roles{models.RoleEnumModify}}
		require.NoError(t, db.User().Create(ctx, &other, ""))
		otherCtx := models.WithUserID(ctx, other.ID)
		guestCtx := models.WithUserID(ctx, 0)
		create := func(c context.Context, favorite bool) int {
			item := models.NewImage()
			item.Favorite = favorite
			item.Title = "favorite test"
			require.NoError(t, qb.Create(c, &models.CreateImageInput{Image: &item}))
			return item.ID
		}
		first := create(ctx, true)
		second := create(ctx, false)
		absent := create(guestCtx, false)
		// Full create, full update and partial update all use the current user's row.
		item, err := qb.Find(ctx, first)
		require.NoError(t, err)
		assert.True(t, item.Favorite)
		item.Favorite = false
		require.NoError(t, qb.Update(ctx, item))
		item, err = qb.Find(ctx, first)
		require.NoError(t, err)
		assert.False(t, item.Favorite)
		_, err = qb.UpdatePartial(ctx, first, models.ImagePartial{Favorite: models.NewOptionalBool(true)})
		require.NoError(t, err)
		_, err = qb.UpdatePartial(otherCtx, second, models.ImagePartial{Favorite: models.NewOptionalBool(true)})
		require.NoError(t, err)
		// Updating the rating leaves an existing favorite intact.
		_, err = qb.UpdatePartial(ctx, first, models.ImagePartial{Rating: models.NewOptionalInt(60)})
		require.NoError(t, err)
		for _, c := range []context.Context{ctx, otherCtx, guestCtx} {
			for _, id := range []int{first, second, absent} {
				item, err := qb.Find(c, id)
				require.NoError(t, err)
				assert.Equal(t, c == ctx && id == first || c == otherCtx && id == second, item.Favorite)
			}
		}
		query := func(c context.Context, favorite *bool, direction models.SortDirectionEnum) []int {
			sort := "favorite"
			perPage := -1
			find := &models.FindFilterType{Sort: &sort, Direction: &direction, PerPage: &perPage}
			filter := &models.ImageFilterType{ID: &models.IntCriterionInput{Value: first, Value2: &absent, Modifier: models.CriterionModifierBetween}, Favorite: favorite}
			var ids []int
			for _, item := range queryImages(c, t, qb, filter, find) {
				ids = append(ids, item.ID)
			}
			return ids
		}
		yes, no := true, false
		for _, tc := range []struct {
			c        context.Context
			favorite int
			others   []int
		}{
			{ctx, first, []int{second, absent}},
			{otherCtx, second, []int{first, absent}},
		} {
			assert.Equal(t, []int{tc.favorite}, query(tc.c, &yes, models.SortDirectionEnumAsc))
			assert.ElementsMatch(t, tc.others, query(tc.c, &no, models.SortDirectionEnumAsc))
			asc := query(tc.c, nil, models.SortDirectionEnumAsc)
			desc := query(tc.c, nil, models.SortDirectionEnumDesc)
			require.Len(t, asc, 3)
			require.Len(t, desc, 3)
			assert.Equal(t, tc.favorite, asc[2])
			assert.Equal(t, tc.favorite, desc[0])
		}
		assert.Empty(t, query(guestCtx, &yes, models.SortDirectionEnumAsc))
		assert.ElementsMatch(t, []int{first, second, absent}, query(guestCtx, &no, models.SortDirectionEnumAsc))
		return nil
	}))
}

func TestGalleryFavorite(t *testing.T) {
	require.NoError(t, withRollbackTxn(func(ctx context.Context) error {
		qb := db.Gallery()
		other := models.User{Username: "gallery-favorites", Roles: models.Roles{models.RoleEnumModify}}
		require.NoError(t, db.User().Create(ctx, &other, ""))
		otherCtx := models.WithUserID(ctx, other.ID)
		guestCtx := models.WithUserID(ctx, 0)
		create := func(c context.Context, favorite bool) int {
			item := models.NewGallery()
			item.Favorite = favorite
			item.Title = "favorite test"
			require.NoError(t, qb.Create(c, &models.CreateGalleryInput{Gallery: &item}))
			return item.ID
		}
		first := create(ctx, true)
		second := create(ctx, false)
		absent := create(guestCtx, false)
		// Full create, full update and partial update all use the current user's row.
		item, err := qb.Find(ctx, first)
		require.NoError(t, err)
		assert.True(t, item.Favorite)
		item.Favorite = false
		require.NoError(t, qb.Update(ctx, &models.UpdateGalleryInput{Gallery: item}))
		item, err = qb.Find(ctx, first)
		require.NoError(t, err)
		assert.False(t, item.Favorite)
		_, err = qb.UpdatePartial(ctx, first, models.GalleryPartial{Favorite: models.NewOptionalBool(true)})
		require.NoError(t, err)
		_, err = qb.UpdatePartial(otherCtx, second, models.GalleryPartial{Favorite: models.NewOptionalBool(true)})
		require.NoError(t, err)
		// Updating the rating leaves an existing favorite intact.
		_, err = qb.UpdatePartial(ctx, first, models.GalleryPartial{Rating: models.NewOptionalInt(60)})
		require.NoError(t, err)
		for _, c := range []context.Context{ctx, otherCtx, guestCtx} {
			for _, id := range []int{first, second, absent} {
				item, err := qb.Find(c, id)
				require.NoError(t, err)
				assert.Equal(t, c == ctx && id == first || c == otherCtx && id == second, item.Favorite)
			}
		}
		query := func(c context.Context, favorite *bool, direction models.SortDirectionEnum) []int {
			sort := "favorite"
			perPage := -1
			find := &models.FindFilterType{Sort: &sort, Direction: &direction, PerPage: &perPage}
			filter := &models.GalleryFilterType{ID: &models.IntCriterionInput{Value: first, Value2: &absent, Modifier: models.CriterionModifierBetween}, Favorite: favorite}
			var ids []int
			for _, item := range queryGallery(c, t, qb, filter, find) {
				ids = append(ids, item.ID)
			}
			return ids
		}
		yes, no := true, false
		for _, tc := range []struct {
			c        context.Context
			favorite int
			others   []int
		}{
			{ctx, first, []int{second, absent}},
			{otherCtx, second, []int{first, absent}},
		} {
			assert.Equal(t, []int{tc.favorite}, query(tc.c, &yes, models.SortDirectionEnumAsc))
			assert.ElementsMatch(t, tc.others, query(tc.c, &no, models.SortDirectionEnumAsc))
			asc := query(tc.c, nil, models.SortDirectionEnumAsc)
			desc := query(tc.c, nil, models.SortDirectionEnumDesc)
			require.Len(t, asc, 3)
			require.Len(t, desc, 3)
			assert.Equal(t, tc.favorite, asc[2])
			assert.Equal(t, tc.favorite, desc[0])
		}
		assert.Empty(t, query(guestCtx, &yes, models.SortDirectionEnumAsc))
		assert.ElementsMatch(t, []int{first, second, absent}, query(guestCtx, &no, models.SortDirectionEnumAsc))
		return nil
	}))
}

func TestGroupFavorite(t *testing.T) {
	require.NoError(t, withRollbackTxn(func(ctx context.Context) error {
		qb := db.Group()
		other := models.User{Username: "group-favorites", Roles: models.Roles{models.RoleEnumModify}}
		require.NoError(t, db.User().Create(ctx, &other, ""))
		otherCtx := models.WithUserID(ctx, other.ID)
		guestCtx := models.WithUserID(ctx, 0)
		create := func(c context.Context, favorite bool) int {
			item := models.NewGroup()
			item.Favorite = favorite
			item.Name = "favorite test"
			require.NoError(t, qb.Create(c, &item))
			return item.ID
		}
		first := create(ctx, true)
		second := create(ctx, false)
		absent := create(guestCtx, false)
		// Full create, full update and partial update all use the current user's row.
		item, err := qb.Find(ctx, first)
		require.NoError(t, err)
		assert.True(t, item.Favorite)
		item.Favorite = false
		require.NoError(t, qb.Update(ctx, item))
		item, err = qb.Find(ctx, first)
		require.NoError(t, err)
		assert.False(t, item.Favorite)
		_, err = qb.UpdatePartial(ctx, first, models.GroupPartial{Favorite: models.NewOptionalBool(true)})
		require.NoError(t, err)
		_, err = qb.UpdatePartial(otherCtx, second, models.GroupPartial{Favorite: models.NewOptionalBool(true)})
		require.NoError(t, err)
		// Updating the rating leaves an existing favorite intact.
		_, err = qb.UpdatePartial(ctx, first, models.GroupPartial{Rating: models.NewOptionalInt(60)})
		require.NoError(t, err)
		for _, c := range []context.Context{ctx, otherCtx, guestCtx} {
			for _, id := range []int{first, second, absent} {
				item, err := qb.Find(c, id)
				require.NoError(t, err)
				assert.Equal(t, c == ctx && id == first || c == otherCtx && id == second, item.Favorite)
			}
		}
		query := func(c context.Context, favorite *bool, direction models.SortDirectionEnum) []int {
			sort := "favorite"
			perPage := -1
			find := &models.FindFilterType{Sort: &sort, Direction: &direction, PerPage: &perPage}
			filter := &models.GroupFilterType{Name: &models.StringCriterionInput{Value: "favorite test", Modifier: models.CriterionModifierEquals}, Favorite: favorite}
			var ids []int
			for _, item := range queryGroups(c, t, filter, find) {
				ids = append(ids, item.ID)
			}
			return ids
		}
		yes, no := true, false
		for _, tc := range []struct {
			c        context.Context
			favorite int
			others   []int
		}{
			{ctx, first, []int{second, absent}},
			{otherCtx, second, []int{first, absent}},
		} {
			assert.Equal(t, []int{tc.favorite}, query(tc.c, &yes, models.SortDirectionEnumAsc))
			assert.ElementsMatch(t, tc.others, query(tc.c, &no, models.SortDirectionEnumAsc))
			asc := query(tc.c, nil, models.SortDirectionEnumAsc)
			desc := query(tc.c, nil, models.SortDirectionEnumDesc)
			require.Len(t, asc, 3)
			require.Len(t, desc, 3)
			assert.Equal(t, tc.favorite, asc[2])
			assert.Equal(t, tc.favorite, desc[0])
		}
		assert.Empty(t, query(guestCtx, &yes, models.SortDirectionEnumAsc))
		assert.ElementsMatch(t, []int{first, second, absent}, query(guestCtx, &no, models.SortDirectionEnumAsc))
		return nil
	}))
}

func TestAudioFavorite(t *testing.T) {
	require.NoError(t, withRollbackTxn(func(ctx context.Context) error {
		qb := db.Audio()
		other := models.User{Username: "audio-favorites", Roles: models.Roles{models.RoleEnumModify}}
		require.NoError(t, db.User().Create(ctx, &other, ""))
		otherCtx := models.WithUserID(ctx, other.ID)
		guestCtx := models.WithUserID(ctx, 0)
		create := func(c context.Context, favorite bool) int {
			item := models.NewAudio()
			item.Favorite = favorite
			item.Title = "favorite test"
			require.NoError(t, qb.Create(c, &item, nil))
			return item.ID
		}
		first := create(ctx, true)
		second := create(ctx, false)
		absent := create(guestCtx, false)
		// Full create, full update and partial update all use the current user's row.
		item, err := qb.Find(ctx, first)
		require.NoError(t, err)
		assert.True(t, item.Favorite)
		item.Favorite = false
		require.NoError(t, qb.Update(ctx, item))
		item, err = qb.Find(ctx, first)
		require.NoError(t, err)
		assert.False(t, item.Favorite)
		_, err = qb.UpdatePartial(ctx, first, models.AudioPartial{Favorite: models.NewOptionalBool(true)})
		require.NoError(t, err)
		_, err = qb.UpdatePartial(otherCtx, second, models.AudioPartial{Favorite: models.NewOptionalBool(true)})
		require.NoError(t, err)
		// Updating the rating leaves an existing favorite intact.
		_, err = qb.UpdatePartial(ctx, first, models.AudioPartial{Rating: models.NewOptionalInt(60)})
		require.NoError(t, err)
		for _, c := range []context.Context{ctx, otherCtx, guestCtx} {
			for _, id := range []int{first, second, absent} {
				item, err := qb.Find(c, id)
				require.NoError(t, err)
				assert.Equal(t, c == ctx && id == first || c == otherCtx && id == second, item.Favorite)
			}
		}
		query := func(c context.Context, favorite *bool, direction models.SortDirectionEnum) []int {
			sort := "favorite"
			perPage := -1
			find := &models.FindFilterType{Sort: &sort, Direction: &direction, PerPage: &perPage}
			filter := &models.AudioFilterType{ID: &models.IntCriterionInput{Value: first, Value2: &absent, Modifier: models.CriterionModifierBetween}, Favorite: favorite}
			var ids []int
			for _, item := range queryAudio(c, t, qb, filter, find) {
				ids = append(ids, item.ID)
			}
			return ids
		}
		yes, no := true, false
		for _, tc := range []struct {
			c        context.Context
			favorite int
			others   []int
		}{
			{ctx, first, []int{second, absent}},
			{otherCtx, second, []int{first, absent}},
		} {
			assert.Equal(t, []int{tc.favorite}, query(tc.c, &yes, models.SortDirectionEnumAsc))
			assert.ElementsMatch(t, tc.others, query(tc.c, &no, models.SortDirectionEnumAsc))
			asc := query(tc.c, nil, models.SortDirectionEnumAsc)
			desc := query(tc.c, nil, models.SortDirectionEnumDesc)
			require.Len(t, asc, 3)
			require.Len(t, desc, 3)
			assert.Equal(t, tc.favorite, asc[2])
			assert.Equal(t, tc.favorite, desc[0])
		}
		assert.Empty(t, query(guestCtx, &yes, models.SortDirectionEnumAsc))
		assert.ElementsMatch(t, []int{first, second, absent}, query(guestCtx, &no, models.SortDirectionEnumAsc))
		return nil
	}))
}

func TestGroupFavoriteExportImport(t *testing.T) {
	require.NoError(t, withRollbackTxn(func(ctx context.Context) error {
		other := models.User{Username: "favorite-import", Roles: models.Roles{models.RoleEnumModify}}
		require.NoError(t, db.User().Create(ctx, &other, ""))
		otherCtx := models.WithUserID(ctx, other.ID)
		original := models.NewGroup()
		original.Name = "favorite round trip"
		original.Favorite = true
		require.NoError(t, db.Group().Create(ctx, &original))
		for _, tc := range []struct {
			c        context.Context
			favorite bool
		}{{ctx, true}, {otherCtx, false}} {
			item, err := db.Group().Find(tc.c, original.ID)
			require.NoError(t, err)
			require.NoError(t, item.LoadURLs(tc.c, db.Group()))
			exported, err := group.ToJSON(tc.c, db.Group(), db.Studio(), item)
			require.NoError(t, err)
			assert.Equal(t, tc.favorite, exported.Favorite, "exports only the current user's favorite")
			data, err := json.Marshal(exported)
			require.NoError(t, err)
			var decoded jsonschema.Group
			require.NoError(t, json.Unmarshal(data, &decoded))
			decoded.Name += " imported"
			importer := group.Importer{ReaderWriter: db.Group(), StudioWriter: db.Studio(), TagWriter: db.Tag(), Input: decoded}
			require.NoError(t, importer.PreImport(otherCtx))
			id, err := importer.Create(otherCtx)
			require.NoError(t, err)
			imported, err := db.Group().Find(otherCtx, *id)
			require.NoError(t, err)
			assert.Equal(t, tc.favorite, imported.Favorite)
			imported, err = db.Group().Find(ctx, *id)
			require.NoError(t, err)
			assert.False(t, imported.Favorite, "import leaves other users alone")
		}
		return nil
	}))
}
