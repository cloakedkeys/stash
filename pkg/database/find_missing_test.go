//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findMissingCase looks up one existing item and one missing item.
type findMissingCase struct {
	name     string
	existing func() int
	findMany func(ctx context.Context, ids []int) (int, error)
	find     func(ctx context.Context, id int) (bool, error)
}

func findMissingOf[T any](
	findMany func(context.Context, []int) ([]*T, error),
	find func(context.Context, int) (*T, error),
) (func(context.Context, []int) (int, error), func(context.Context, int) (bool, error)) {
	return func(ctx context.Context, ids []int) (int, error) {
			got, err := findMany(ctx, ids)
			return len(got), err
		}, func(ctx context.Context, id int) (bool, error) {
			got, err := find(ctx, id)
			return got != nil, err
		}
}

// TestFindManyMissingThenFind checks what Generate relies on to skip items
// that no longer exist (#2092): FindMany fails when an item is missing, and
// Find, in the same transaction, then finds the items that exist and returns
// nothing, without an error, for the missing ones.
func TestFindManyMissingThenFind(t *testing.T) {
	var tests []findMissingCase
	add := func(name string, existing func() int, findMany func(context.Context, []int) (int, error), find func(context.Context, int) (bool, error)) {
		tests = append(tests, findMissingCase{name, existing, findMany, find})
	}

	fm, f := findMissingOf(db.Scene().FindMany, db.Scene().Find)
	add("scene", func() int { return sceneIDs[0] }, fm, f)
	fm, f = findMissingOf(db.SceneMarker().FindMany, db.SceneMarker().Find)
	add("scene marker", func() int { return markerIDs[0] }, fm, f)
	fm, f = findMissingOf(db.Image().FindMany, db.Image().Find)
	add("image", func() int { return imageIDs[0] }, fm, f)
	fm, f = findMissingOf(db.Gallery().FindMany, db.Gallery().Find)
	add("gallery", func() int { return galleryIDs[0] }, fm, f)

	for _, tt := range tests {
		runWithRollbackTxn(t, tt.name, func(t *testing.T, ctx context.Context) {
			existing := tt.existing()

			_, err := tt.findMany(ctx, []int{existing, invalidID})
			require.Error(t, err)

			found, err := tt.find(ctx, existing)
			require.NoError(t, err)
			assert.True(t, found, "existing item")

			found, err = tt.find(ctx, invalidID)
			require.NoError(t, err)
			assert.False(t, found, "missing item")

			n, err := tt.findMany(ctx, []int{existing})
			require.NoError(t, err)
			assert.Equal(t, 1, n)
		})
	}
}
