package models

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

type CustomFieldMap map[string]interface{}

// CustomFieldRegexPattern returns the text of a custom field regex filter value.
// The UI sends anything that looks like a number as a number, so a number is
// used as the pattern written as text: 5 as "5", 1.5 as "1.5". Numbers arrive as
// json.Number from GraphQL variables and as int64 or float64 from literals or
// after conversion. Any other value, such as a bool, a list or null, is an error.
func CustomFieldRegexPattern(v interface{}) (string, error) {
	switch v := v.(type) {
	case string:
		return v, nil
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return strconv.FormatInt(i, 10), nil
		}
		f, err := v.Float64()
		if err != nil {
			return "", fmt.Errorf("invalid custom field regex number %q: %w", v, err)
		}
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("custom field regex value must be text or a number, got %T", v)
	}
}

type CustomFieldsInput struct {
	// If populated, the entire custom fields map will be replaced with this value
	Full map[string]interface{} `json:"full"`
	// If populated, only the keys in this map will be updated
	Partial map[string]interface{} `json:"partial"`
	// Remove any keys in this list
	Remove []string `json:"remove"`
}

type CustomFieldsReader interface {
	GetCustomFields(ctx context.Context, id int) (map[string]interface{}, error)
	GetCustomFieldsBulk(ctx context.Context, ids []int) ([]CustomFieldMap, error)
}

type CustomFieldsWriter interface {
	SetCustomFields(ctx context.Context, id int, fields CustomFieldsInput) error
}
