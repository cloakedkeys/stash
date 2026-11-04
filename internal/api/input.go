package api

import (
	"fmt"
	"strconv"

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

// validateIDList returns an error if there are any duplicate ids in the list.
// Ids that are numbers are compared by value, so "1" and "01" are the same id.
// Ids that are not numbers are compared as written; converting them fails later.
func validateIDList(ids []string) error {
	seenNumbers := make(map[int]struct{})
	seenOthers := make(map[string]struct{})
	for _, id := range ids {
		exists := false
		if n, err := strconv.Atoi(id); err == nil {
			_, exists = seenNumbers[n]
			seenNumbers[n] = struct{}{}
		} else {
			_, exists = seenOthers[id]
			seenOthers[id] = struct{}{}
		}

		if exists {
			return fmt.Errorf("duplicate id found: %s", id)
		}
	}
	return nil
}
