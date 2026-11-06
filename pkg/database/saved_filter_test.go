//go:build integration
// +build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSavedFilterFind(t *testing.T) {
	runTxn(t, func(ctx context.Context) error {
		savedFilter, err := db.SavedFilter().Find(ctx, savedFilterIDs[savedFilterIdxImage])

		if err != nil {
			t.Errorf("Error finding saved filter: %s", err.Error())
		}

		assert.Equal(t, savedFilterIDs[savedFilterIdxImage], savedFilter.ID)

		return nil
	})
}

func TestSavedFilterFindByMode(t *testing.T) {
	runTxn(t, func(ctx context.Context) error {
		savedFilters, err := db.SavedFilter().FindByMode(ctx, models.FilterModeScenes)

		if err != nil {
			t.Errorf("Error finding saved filters: %s", err.Error())
		}

		assert.Len(t, savedFilters, 1)
		assert.Equal(t, savedFilterIDs[savedFilterIdxScene], savedFilters[0].ID)

		return nil
	})
}

func TestSavedFilterDestroy(t *testing.T) {
	const filterName = "filterToDestroy"
	filterQ := ""
	filterPage := 1
	filterPerPage := 40
	filterSort := "date"
	filterDirection := models.SortDirectionEnumAsc
	findFilter := models.FindFilterType{
		Q:         &filterQ,
		Page:      &filterPage,
		PerPage:   &filterPerPage,
		Sort:      &filterSort,
		Direction: &filterDirection,
	}
	objectFilter := map[string]interface{}{
		"test": "foo",
	}
	uiOptions := map[string]interface{}{
		"display_mode": 1,
		"zoom_index":   1,
	}
	var id int

	// create the saved filter to destroy
	runTxn(t, func(ctx context.Context) error {
		newFilter := models.SavedFilter{
			Name:         filterName,
			Mode:         models.FilterModeScenes,
			FindFilter:   &findFilter,
			ObjectFilter: objectFilter,
			UIOptions:    uiOptions,
		}
		err := db.SavedFilter().Create(ctx, &newFilter)

		if err == nil {
			id = newFilter.ID
		}

		return err
	})

	runTxn(t, func(ctx context.Context) error {
		return db.SavedFilter().Destroy(ctx, id)
	})

	// now try to find it
	runTxn(t, func(ctx context.Context) error {
		found, err := db.SavedFilter().Find(ctx, id)
		if err == nil {
			assert.Nil(t, found)
		}

		return err
	})
}

// TODO Update
// TODO Destroy
// TODO Find
// TODO GetMarkerStrings
// TODO Wall
// TODO Query

func TestSavedFilterPerUser(t *testing.T) {
	runRollbackTxn(t, func(ctx context.Context) error {
		qb := db.SavedFilter()

		other := models.User{
			Username: "other",
			Roles:    models.Roles{models.RoleEnumModify},
		}
		require.NoError(t, db.User().Create(ctx, &other, ""))
		otherCtx := models.WithUserID(ctx, other.ID)
		guestCtx := models.WithUserID(ctx, 0)

		const name = "per user filter"
		newFilter := models.SavedFilter{
			Name: name,
			Mode: models.FilterModeScenes,
		}
		require.NoError(t, qb.Create(ctx, &newFilter))

		// other users may use the same name
		otherFilter := models.SavedFilter{
			Name: name,
			Mode: models.FilterModeScenes,
		}
		require.NoError(t, qb.Create(otherCtx, &otherFilter))

		// the guest user cannot create filters
		assert.Error(t, qb.Create(guestCtx, &models.SavedFilter{Name: name, Mode: models.FilterModeScenes}))

		// users only see their own filters
		f, err := qb.Find(otherCtx, newFilter.ID)
		require.NoError(t, err)
		assert.Nil(t, f)

		f, err = qb.Find(guestCtx, newFilter.ID)
		require.NoError(t, err)
		assert.Nil(t, f)

		all, err := qb.All(otherCtx)
		require.NoError(t, err)
		require.Len(t, all, 1)
		assert.Equal(t, otherFilter.ID, all[0].ID)

		byMode, err := qb.FindByMode(guestCtx, models.FilterModeScenes)
		require.NoError(t, err)
		assert.Empty(t, byMode)

		// users cannot change or remove other users' filters
		assert.Error(t, qb.Update(otherCtx, &models.SavedFilter{ID: newFilter.ID, Name: "changed", Mode: models.FilterModeScenes}))
		assert.Error(t, qb.Destroy(otherCtx, newFilter.ID))

		f, err = qb.Find(ctx, newFilter.ID)
		require.NoError(t, err)
		require.NotNil(t, f)
		assert.Equal(t, name, f.Name)

		return nil
	})
}

// All lists a user's saved filters by mode and then name, and FindByMode by
// name, in the same order on both databases, whatever order they were created
// or changed in. The order is SQLite's: by bytes, so capitals come before
// lower case, _ after capitals, accented letters last, and SCENES before
// SCENE_MARKERS.
func TestSavedFilterAllOrder(t *testing.T) {
	require.NoError(t, withRollbackTxn(func(ctx context.Context) error {
		qb := db.SavedFilter()

		owner := models.User{
			Username: "filter order",
			Roles:    models.Roles{models.RoleEnumModify},
		}
		require.NoError(t, db.User().Create(ctx, &owner, ""))
		ctx = models.WithUserID(ctx, owner.ID)

		var changed *models.SavedFilter
		for _, f := range []struct {
			mode models.FilterMode
			name string
		}{
			{models.FilterModeScenes, "b"},
			{models.FilterModeSceneMarkers, "m"},
			{models.FilterModeImages, "z"},
			{models.FilterModeScenes, "é"},
			{models.FilterModeScenes, "a"},
			{models.FilterModeScenes, "_x"},
			{models.FilterModeGalleries, "m"},
			{models.FilterModeScenes, "e"},
			{models.FilterModeScenes, "B"},
		} {
			sf := &models.SavedFilter{Mode: f.mode, Name: f.name}
			require.NoError(t, qb.Create(ctx, sf))
			if f.mode == models.FilterModeScenes && f.name == "b" {
				changed = sf
			}
		}

		// changing a filter doesn't move it
		changed.UIOptions = map[string]interface{}{"zoom_index": 2}
		require.NoError(t, qb.Update(ctx, changed))

		names := func(filters []*models.SavedFilter, withMode bool) []string {
			var ret []string
			for _, f := range filters {
				if withMode {
					ret = append(ret, string(f.Mode)+" "+f.Name)
				} else {
					ret = append(ret, f.Name)
				}
			}
			return ret
		}

		all, err := qb.All(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"GALLERIES m",
			"IMAGES z",
			"SCENES B",
			"SCENES _x",
			"SCENES a",
			"SCENES b",
			"SCENES e",
			"SCENES é",
			"SCENE_MARKERS m",
		}, names(all, true))

		scenes, err := qb.FindByMode(ctx, models.FilterModeScenes)
		require.NoError(t, err)
		assert.Equal(t, []string{"B", "_x", "a", "b", "e", "é"}, names(scenes, false))

		return nil
	}))
}
