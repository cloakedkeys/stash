//go:build integration
// +build integration

package database_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2224: the Go code that follows the parent folders had no check for a
// loop, so a folder that ends up as its own ancestor made it run forever. It
// must stop at a folder it has already seen.
func TestFolderParentLoop(t *testing.T) {
	runWithRollbackTxn(t, "parent folders end on a loop", func(t *testing.T, ctx context.Context) {
		// A and B are each other's parent, C is under B
		root := filepath.Join(t.TempDir(), "folder loop")
		a := &models.Folder{Path: filepath.Join(root, "A")}
		require.NoError(t, db.Folder().Create(ctx, a))
		b := &models.Folder{Path: filepath.Join(root, "A", "B"), ParentFolderID: &a.ID}
		require.NoError(t, db.Folder().Create(ctx, b))
		c := &models.Folder{Path: filepath.Join(root, "A", "B", "C"), ParentFolderID: &b.ID}
		require.NoError(t, db.Folder().Create(ctx, c))
		// the store refuses nothing here; an old bug or a bad import can leave this
		_, _, err := db.ExecSQL(ctx, fmt.Sprintf("UPDATE folders SET parent_folder_id = %d WHERE id = %d", b.ID, a.ID), nil)
		require.NoError(t, err)

		type result struct {
			got [][]models.FolderID
			err error
		}
		done := make(chan result, 1)
		go func() {
			got, err := db.Folder().GetManyParentFolderIDs(ctx, []models.FolderID{c.ID, a.ID, b.ID})
			done <- result{got, err}
		}()

		// the walk in Go doesn't watch the context, so wait here instead. Keep
		// the wait short: a walk that doesn't end keeps growing its list.
		select {
		case r := <-done:
			require.NoError(t, r.err)
			assert.Equal(t, [][]models.FolderID{
				{b.ID, a.ID},
				{b.ID},
				{a.ID},
			}, r.got)
		case <-time.After(time.Second):
			t.Fatal("the parent folder walk did not end on a loop")
		}
	})
}
