//go:build integration
// +build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stashapp/stash/pkg/tag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2218: a tag import set the parents it was given without the loop check
// that tag edits use, so an export where two tags are each other's parent
// saved a loop. The import keeps the tag and its other parents, and leaves
// out only the parent that would make the loop.
func TestTagImportParentLoop(t *testing.T) {
	runWithRollbackTxn(t, "import leaves out a parent that makes a loop", func(t *testing.T, ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		tqb := db.Tag()
		ids := map[string]int{}
		for _, n := range []string{"A", "B", "Other"} {
			tg := models.Tag{Name: "import loop " + n}
			require.NoError(t, tqb.Create(ctx, &models.CreateTagInput{Tag: &tg}))
			ids[n] = tg.ID
		}
		// an earlier import already made B a child of A
		require.NoError(t, tqb.UpdateParentTags(ctx, ids["B"], []int{ids["A"]}))

		// now A comes in with parents B (under A already), Other, and itself
		i := tag.Importer{
			ReaderWriter:        tqb,
			MissingRefBehaviour: models.ImportMissingRefEnumFail,
			Input: jsonschema.Tag{
				Name:    "import loop A",
				Parents: []string{"import loop B", "import loop Other", "import loop A"},
			},
		}
		require.NoError(t, i.PostImport(ctx, ids["A"]))

		parents, err := tqb.GetParentIDs(ctx, ids["A"])
		require.NoError(t, err)
		assert.Equal(t, []int{ids["Other"]}, parents, "A keeps only the parent that makes no loop")

		parents, err = tqb.GetParentIDs(ctx, ids["B"])
		require.NoError(t, err)
		assert.Equal(t, []int{ids["A"]}, parents, "B is unchanged")
	})
}
