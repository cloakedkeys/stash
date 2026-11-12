//go:build integration

package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/require"
)

func TestFindDuplicateScenesByStashID(t *testing.T) {
	ctx := context.Background()
	db := openUserDataTestDatabase(ctx, t)
	user := models.User{Username: "duplicate-reader", Roles: models.Roles{models.RoleEnumRead}}
	ids := make([]int, 3)
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		if err := db.User().Create(ctx, &user, ""); err != nil {
			return err
		}
		for i := range ids {
			s := models.NewScene()
			s.Organized = i == 2
			s.StashIDs = models.NewRelatedStashIDs([]models.StashID{{Endpoint: "https://one.example/graphql", StashID: "shared"}})
			if err := db.Scene().Create(ctx, &s, nil); err != nil {
				return err
			}
			ids[i] = s.ID
		}
		return nil
	}))
	r := &Resolver{repository: db.Repository()}
	srv := newTestGraphQLServer(r)
	query := `query($stashID: Boolean) { findDuplicateScenes(stash_id:$stashID, distance:0, duration_diff:0, scene_filter:{organized:false}) { id } }`
	body := customFieldsRequest(t, srv, user, query, `{"stashID":true}`)
	require.JSONEq(t, fmt.Sprintf(`{"data":{"findDuplicateScenes":[[{"id":"%d"},{"id":"%d"}]]}}`, ids[0], ids[1]), body)
	for _, variables := range []string{`{}`, `{"stashID":false}`, `{"stashID":null}`} {
		body = customFieldsRequest(t, srv, user, query, variables)
		require.JSONEq(t, `{"data":{"findDuplicateScenes":[]}}`, body)
	}
}
