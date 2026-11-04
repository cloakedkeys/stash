package api

import (
	"context"
	"fmt"

	"github.com/stashapp/stash/pkg/models"
)

// linkedObjectExists returns a not-found error, such as "studio with id 999 not
// found", when an edit links to an object that doesn't exist, such as a scene's
// studio. Without this the database refused the edit with its own foreign key
// error, which shows its command. There is nothing to check when the edit leaves
// the link out or clears it. Run it in the same transaction as the write.
func linkedObjectExists[T any](ctx context.Context, find func(context.Context, int) (*T, error), kind string, v models.OptionalInt) error {
	if !v.Set || v.Null {
		return nil
	}
	return userDataObjectExists(ctx, find, kind, v.Value)
}

// listedOnce refuses an id given more than once, such as "tag with id 2 is listed
// more than once", for calls that refused a repeat with a database error.
func listedOnce(kind string, ids []int) error {
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			return fmt.Errorf("%s with id %d is listed more than once", kind, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

// linkedStudioExists is linkedObjectExists for a studio_id or a parent_id.
func (r *Resolver) linkedStudioExists(ctx context.Context, v models.OptionalInt) error {
	return linkedObjectExists(ctx, r.repository.Studio.Find, "studio", v)
}
