package api

import (
	"encoding/json"
	"fmt"

	"github.com/stashapp/stash/pkg/models"
)

// convertMapJSONNumbers converts all JSON numbers in a map, and in maps nested
// in it, to either int64 or float64 as models.JSONNumberValue does. The error
// names the key of a number that cannot be converted, such as 1e400.
func convertMapJSONNumbers(m map[string]interface{}) (map[string]interface{}, error) {
	if m == nil {
		return nil, nil
	}

	ret := make(map[string]interface{})
	for k, v := range m {
		if n, ok := v.(json.Number); ok {
			num, err := models.JSONNumberValue(n)
			if err != nil {
				return nil, fmt.Errorf("%q: %w", k, err)
			}
			ret[k] = num
		} else if mm, ok := v.(map[string]interface{}); ok {
			converted, err := convertMapJSONNumbers(mm)
			if err != nil {
				return nil, fmt.Errorf("%q.%w", k, err)
			}
			ret[k] = converted
		} else {
			ret[k] = v
		}
	}

	return ret, nil
}
