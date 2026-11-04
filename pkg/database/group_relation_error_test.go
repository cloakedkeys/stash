//go:build integration

package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/require"
)

// TestGroupRelationInsertErrorNamesTableOnce checks that a failed sub-group
// insert names its table once, not "inserting into groups_relations: inserting
// into groups_relations: ...". A read-only transaction makes the insert fail.
func TestGroupRelationInsertErrorNamesTableOnce(t *testing.T) {
	var err error
	// the transaction's own result is the same error, or on PostgreSQL its rollback
	_ = txn.WithReadTxn(context.Background(), db, func(ctx context.Context) error {
		err = db.Group().AddSubGroups(ctx, groupIDs[groupIdxWithScene], []models.GroupIDDescription{{GroupID: groupIDs[groupIdxWithStudio]}}, nil)
		return err
	})
	require.Error(t, err)
	require.Equal(t, 1, strings.Count(err.Error(), "inserting into groups_relations"), err.Error())
}
