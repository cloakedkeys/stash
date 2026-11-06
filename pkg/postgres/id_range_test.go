//go:build integration

package postgres

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDatabase(t *testing.T) *Database {
	t.Helper()
	ctx := context.Background()
	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, migrationFixture(t)))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

// rolledBack runs f in a transaction that is rolled back. f must not fail the test, as that
// would leave the transaction open.
func rolledBack(t *testing.T, db *Database, f func(ctx context.Context) error) {
	t.Helper()
	err := txn.WithTxn(models.WithUserID(context.Background(), 1), db, func(ctx context.Context) error {
		if err := f(ctx); err != nil {
			return err
		}
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)
}

// An id too large for the 32-bit id columns matches nothing, as it does on SQLite. Looking one
// up finds nothing, rather than failing because the id can't be sent as a parameter.
func TestIDOutOfRangeIsNotFound(t *testing.T) {
	db := openTestDatabase(t)

	for _, id := range []int{3000000000, -3000000000} {
		type result struct {
			name  string
			value interface{}
			err   error
		}
		var results []result
		var after error
		rolledBack(t, db, func(ctx context.Context) error {
			add := func(name string, v interface{}, err error) {
				results = append(results, result{name, v, err})
			}
			v1, err := db.Scene().Find(ctx, id)
			add("scene", v1, err)
			v2, err := db.Image().Find(ctx, id)
			add("image", v2, err)
			v3, err := db.Gallery().Find(ctx, id)
			add("gallery", v3, err)
			v4, err := db.Performer().Find(ctx, id)
			add("performer", v4, err)
			v5, err := db.Studio().Find(ctx, id)
			add("studio", v5, err)
			v6, err := db.Tag().Find(ctx, id)
			add("tag", v6, err)
			v7, err := db.Group().Find(ctx, id)
			add("group", v7, err)
			v8, err := db.SceneMarker().Find(ctx, id)
			add("marker", v8, err)
			v9, err := db.GalleryChapter().Find(ctx, id)
			add("chapter", v9, err)
			v10, err := db.SavedFilter().Find(ctx, id)
			add("saved filter", v10, err)
			v11, err := db.Audio().Find(ctx, id)
			add("audio", v11, err)
			v12, err := db.Story().Find(ctx, id)
			add("story", v12, err)
			v13, err := db.User().(*UserStore).Find(ctx, id)
			add("user", v13, err)
			v14, err := db.Scene().GetCustomFields(ctx, id)
			add("custom fields", v14, err)
			v15, err := db.Tag().GetParentIDs(ctx, id)
			add("parent tag ids", v15, err)
			v16, err := db.Scene().GetURLs(ctx, id)
			add("urls", v16, err)
			v17, err := db.Performer().GetStashIDs(ctx, id)
			add("stash ids", v17, err)
			v18, err := db.Group().GetAliases(ctx, id)
			add("aliases", v18, err)
			add("destroy", nil, db.Scene().Destroy(ctx, id))

			// the transaction can still be used
			var one int
			after = dbWrapper.Get(ctx, &one, "SELECT 1")
			return nil
		})

		for _, r := range results {
			if r.name == "destroy" {
				var notFound *NotFoundError
				assert.True(t, errors.As(r.err, &notFound), "%d: destroy: %v", id, r.err)
				continue
			}
			assert.NoError(t, r.err, "%d: %s", id, r.name)
			if r.value != nil {
				v := reflect.ValueOf(r.value)
				empty := (v.Kind() == reflect.Ptr && v.IsNil()) || ((v.Kind() == reflect.Slice || v.Kind() == reflect.Map) && v.Len() == 0)
				assert.True(t, empty, "%d: %s found %v", id, r.name, r.value)
			}
		}
		assert.NoError(t, after, "%d: transaction after the lookups", id)
	}
}

// Removing links takes ids the caller gives, any number of them, including ids that can't
// exist.
func TestRemoveJoinsManyAndOutOfRangeIDs(t *testing.T) {
	db := openTestDatabase(t)

	many := make([]int, 70000)
	for i := range many {
		many[i] = 10000000 + i
	}

	for name, remove := range map[string]func(tagID int) []int{
		"70000 ids":    func(tagID int) []int { return append(many, tagID) },
		"out of range": func(tagID int) []int { return []int{3000000000, tagID} },
	} {
		t.Run(name, func(t *testing.T) {
			var removeErr error
			var left []int
			var keepID int
			rolledBack(t, db, func(ctx context.Context) error {
				var tagID, sceneID int
				if err := dbWrapper.Get(ctx, &tagID, "INSERT INTO tags (name, created_at, updated_at) VALUES ('remove', NOW(), NOW()) RETURNING id"); err != nil {
					return err
				}
				if err := dbWrapper.Get(ctx, &keepID, "INSERT INTO tags (name, created_at, updated_at) VALUES ('keep', NOW(), NOW()) RETURNING id"); err != nil {
					return err
				}
				if err := dbWrapper.Get(ctx, &sceneID, "INSERT INTO scenes (title, created_at, updated_at) VALUES ('scene', NOW(), NOW()) RETURNING id"); err != nil {
					return err
				}
				if _, err := db.Scene().UpdatePartial(ctx, sceneID, models.ScenePartial{TagIDs: &models.UpdateIDs{IDs: []int{tagID, keepID}, Mode: models.RelationshipUpdateModeAdd}}); err != nil {
					return err
				}
				_, removeErr = db.Scene().UpdatePartial(ctx, sceneID, models.ScenePartial{TagIDs: &models.UpdateIDs{IDs: remove(tagID), Mode: models.RelationshipUpdateModeRemove}})
				if removeErr != nil {
					return nil
				}
				var err error
				left, err = db.Scene().GetTagIDs(ctx, sceneID)
				return err
			})
			require.NoError(t, removeErr)
			assert.Equal(t, []int{keepID}, left, "only the kept tag is left")
		})
	}
}
