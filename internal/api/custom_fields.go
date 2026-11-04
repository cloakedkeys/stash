package api

import (
	"fmt"

	"github.com/stashapp/stash/pkg/models"
)

// convertCustomFields converts the JSON numbers in a map of custom fields to
// int64 or float64, failing on a number that cannot be stored, such as 1e400.
func convertCustomFields(m map[string]interface{}) (map[string]interface{}, error) {
	ret, err := convertMapJSONNumbers(m)
	if err != nil {
		return nil, fmt.Errorf("custom field %w", err)
	}
	return ret, nil
}

// handleUpdateCustomFields converts the JSON numbers in input as
// convertCustomFields does.
func handleUpdateCustomFields(input models.CustomFieldsInput) (models.CustomFieldsInput, error) {
	ret := input
	var err error
	if ret.Full, err = convertCustomFields(ret.Full); err != nil {
		return ret, err
	}
	if ret.Partial, err = convertCustomFields(ret.Partial); err != nil {
		return ret, err
	}

	return ret, nil
}
