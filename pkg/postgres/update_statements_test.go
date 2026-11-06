//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

var errRollback = errors.New("roll back")

// Updating objects one at a time should reuse the same statements. When the object's id is
// written into the SQL text, every update is a new statement that the driver prepares on the
// server, which costs extra round trips on each update (#2194).
func TestUpdatePartialReusesStatements(t *testing.T) {
	db := openTestDatabase(t)

	const n = 20
	update := map[string]func(ctx context.Context, id, tagID int) error{
		"scenes": func(ctx context.Context, id, tagID int) error {
			_, err := db.Scene().UpdatePartial(ctx, id, models.ScenePartial{TagIDs: &models.UpdateIDs{IDs: []int{tagID}, Mode: models.RelationshipUpdateModeAdd}})
			return err
		},
		"images": func(ctx context.Context, id, tagID int) error {
			_, err := db.Image().UpdatePartial(ctx, id, models.ImagePartial{TagIDs: &models.UpdateIDs{IDs: []int{tagID}, Mode: models.RelationshipUpdateModeSet}})
			return err
		},
		"galleries": func(ctx context.Context, id, tagID int) error {
			_, err := db.Gallery().UpdatePartial(ctx, id, models.GalleryPartial{TagIDs: &models.UpdateIDs{IDs: []int{tagID}, Mode: models.RelationshipUpdateModeAdd}})
			return err
		},
		"performers": func(ctx context.Context, id, tagID int) error {
			_, err := db.Performer().UpdatePartial(ctx, id, models.PerformerPartial{TagIDs: &models.UpdateIDs{IDs: []int{tagID}, Mode: models.RelationshipUpdateModeRemove}})
			return err
		},
	}

	for table, f := range update {
		t.Run(table, func(t *testing.T) {
			col := "title"
			if table == "performers" {
				col = "name"
			}
			// the names of the statements prepared on the connection. Comparing names, not
			// the count, also catches a full cache dropping one statement for each new one.
			const names = "SELECT name FROM pg_prepared_statements ORDER BY name"
			var before, after []string
			rolledBack(t, db, func(ctx context.Context) error {
				var tagID int
				if err := dbWrapper.Get(ctx, &tagID, "INSERT INTO tags (name, created_at, updated_at) VALUES ('tag', NOW(), NOW()) RETURNING id"); err != nil {
					return err
				}
				var ids []int
				if err := dbWrapper.Select(ctx, &ids, "INSERT INTO "+table+" ("+col+", created_at, updated_at) SELECT 'object ' || x, NOW(), NOW() FROM generate_series(1, ?) AS x RETURNING id", n); err != nil {
					return err
				}

				// the first update prepares the statements an update needs, and the
				// first listing prepares the listing query
				if err := f(ctx, ids[0], tagID); err != nil {
					return err
				}
				if err := dbWrapper.Select(ctx, &before, names); err != nil {
					return err
				}
				for _, id := range ids[1:] {
					if err := f(ctx, id, tagID); err != nil {
						return err
					}
				}
				if err := dbWrapper.Select(ctx, &after, names); err != nil {
					return err
				}
				return nil
			})
			require.NotEmpty(t, before)
			require.Equal(t, before, after, "updating %d more %s prepared new statements", n-1, table)
		})
	}
}
