package savedfilter

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
)

// ToJSON converts a SavedFilter object into its JSON equivalent.
func ToJSON(ctx context.Context, filter *models.SavedFilter) (*jsonschema.SavedFilter, error) {
	return &jsonschema.SavedFilter{
		Name:         filter.Name,
		Mode:         filter.Mode,
		FindFilter:   filter.FindFilter,
		ObjectFilter: filter.ObjectFilter,
		UIOptions:    filter.UIOptions,
		// _ and % in this version's filters match only themselves
		LiteralWildcards: true,
	}, nil
}
