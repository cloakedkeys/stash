package api

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/stashapp/stash/pkg/models"
)

// validateMapJSONNumbers returns an error for the first JSON number in m, or in
// the maps and lists nested in it at any depth, that models.JSONNumberValue
// cannot convert, such as 1e400. It changes nothing, so numbers that are fine
// are kept as they were written. The error names where the number is as
// convertMapJSONNumbers does: "key"."nested" for a key of a nested map, and
// "key"[0] for an item of a list.
func validateMapJSONNumbers(m map[string]interface{}) error {
	return validateJSONNumbersAt(m, "")
}

func validateJSONNumbersAt(v interface{}, path string) error {
	switch v := v.(type) {
	case json.Number:
		if _, err := models.JSONNumberValue(v); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	case map[string]interface{}:
		for _, k := range slices.Sorted(maps.Keys(v)) {
			keyPath := strconv.Quote(k)
			if path != "" {
				keyPath = path + "." + keyPath
			}
			if err := validateJSONNumbersAt(v[k], keyPath); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, item := range v {
			if err := validateJSONNumbersAt(item, path+"["+strconv.Itoa(i)+"]"); err != nil {
				return err
			}
		}
	}

	return nil
}
