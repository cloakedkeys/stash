package sqlite

import (
	"context"

	"github.com/stashapp/stash/pkg/librarylimit"
	"github.com/stashapp/stash/pkg/models"
)

// visibleIDs returns the IDs that may be seen in the context's library limit,
// so that a limit isn't mistaken for an object not existing.
func visibleIDs(ctx context.Context, kind librarylimit.Kind, ids []int) ([]int, error) {
	l := models.LibraryLimitFromContext(ctx)
	if l == nil {
		return ids, nil
	}

	query := librarylimit.IDsQuery(l, kind, ids)
	if query == "" {
		return ids, nil
	}

	var found []int
	if err := dbWrapper.Select(ctx, &found, query); err != nil {
		return nil, err
	}

	return librarylimit.KeepVisible(ids, found), nil
}
