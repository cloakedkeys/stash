package api

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/stashapp/stash/pkg/models"
)

// convertMapJSONNumbers converts all JSON numbers in a map, and in the maps and
// lists nested in it at any depth, to either int64 or float64 as
// models.JSONNumberValue does. The error names where a number that cannot be
// converted, such as 1e400, is: "key" for a key of m, "key"."nested" for a key
// of a nested map, and "key"[0] for an item of a list.
func convertMapJSONNumbers(m map[string]interface{}) (map[string]interface{}, error) {
	if m == nil {
		return nil, nil
	}

	return convertMapJSONNumbersAt(m, "")
}

// convertJSONNumbers converts the JSON numbers in v as convertMapJSONNumbers
// does. v may be a number, a map, a list or any other value, which is returned
// unchanged. path names v in the error, and may be empty.
func convertJSONNumbers(v interface{}, path string) (interface{}, error) {
	switch v := v.(type) {
	case json.Number:
		num, err := models.JSONNumberValue(v)
		if err != nil {
			if path == "" {
				return nil, err
			}
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return num, nil
	case map[string]interface{}:
		if v == nil {
			return v, nil
		}
		return convertMapJSONNumbersAt(v, path)
	case []interface{}:
		if v == nil {
			return v, nil
		}
		ret := make([]interface{}, len(v))
		for i, item := range v {
			converted, err := convertJSONNumbers(item, path+"["+strconv.Itoa(i)+"]")
			if err != nil {
				return nil, err
			}
			ret[i] = converted
		}
		return ret, nil
	default:
		return v, nil
	}
}

func convertMapJSONNumbersAt(m map[string]interface{}, path string) (map[string]interface{}, error) {
	ret := make(map[string]interface{}, len(m))
	for k, v := range m {
		keyPath := strconv.Quote(k)
		if path != "" {
			keyPath = path + "." + keyPath
		}

		converted, err := convertJSONNumbers(v, keyPath)
		if err != nil {
			return nil, err
		}
		ret[k] = converted
	}

	return ret, nil
}
