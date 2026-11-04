package api

import (
	"fmt"

	"github.com/stashapp/stash/pkg/sliceutil/stringslice"
)

// TODO - apply handleIDs to other resolvers that accept ID lists

// handleIDList validates and converts a list of string IDs to integers
func handleIDList(idList []string, field string) ([]int, error) {
	if err := validateIDList(idList); err != nil {
		return nil, fmt.Errorf("validating %s: %w", field, err)
	}

	ids, err := stringslice.StringSliceToIntSlice(idList)
	if err != nil {
		return nil, fmt.Errorf("converting %s: %w", field, err)
	}

	return ids, nil
}

// handleFindIDList is handleIDList for the ids of a find query. A find query
// given the same id twice used to fail, so ids that are the same number
// written differently, such as "1" and "01", are rejected as repeats too.
func handleFindIDList(idList []string, field string) ([]int, error) {
	ids, err := handleIDList(idList, field)
	if err != nil {
		return nil, err
	}

	if err := validateIntIDList(ids); err != nil {
		return nil, fmt.Errorf("validating %s: %w", field, err)
	}

	return ids, nil
}

// validateIDList returns an error if there are any duplicate ids in the list
func validateIDList(ids []string) error {
	seen := make(map[string]struct{})
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate id found: %s", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

// validateIntIDList returns an error if any id is in the list more than once.
func validateIntIDList(ids []int) error {
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate id found: %d", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}
