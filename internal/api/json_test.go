package api

import (
	"encoding/json"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestConvertMapJSONNumbers(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "Convert JSON numbers to numbers",
			input: map[string]interface{}{
				"int":    json.Number("12"),
				"float":  json.Number("12.34"),
				"string": "foo",
			},
			expected: map[string]interface{}{
				"int":    int64(12),
				"float":  12.34,
				"string": "foo",
			},
		},
		{
			name: "Convert JSON numbers to numbers in nested maps",
			input: map[string]interface{}{
				"foo": map[string]interface{}{
					"int":           json.Number("56"),
					"float":         json.Number("56.78"),
					"nested-string": "bar",
				},
				"int":    json.Number("12"),
				"float":  json.Number("12.34"),
				"string": "foo",
			},
			expected: map[string]interface{}{
				"foo": map[string]interface{}{
					"int":           int64(56),
					"float":         56.78,
					"nested-string": "bar",
				},
				"int":    int64(12),
				"float":  12.34,
				"string": "foo",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertMapJSONNumbers(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// #2061: a number with an exponent or beyond int64 used to become 0.
func TestConvertMapJSONNumbersBigNumbers(t *testing.T) {
	got, err := convertMapJSONNumbers(map[string]interface{}{
		"exp":      json.Number("1e20"),
		"upperExp": json.Number("1E5"),
		"negExp":   json.Number("-1e3"),
		"decimal":  json.Number("1.5"),
		"int":      json.Number("5"),
		"beyond":   json.Number("99999999999999999999"),
		"nested":   map[string]interface{}{"exp": json.Number("1e20")},
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"exp":      1e20,
		"upperExp": 1e5,
		"negExp":   -1e3,
		"decimal":  1.5,
		"int":      int64(5),
		"beyond":   1e20,
		"nested":   map[string]interface{}{"exp": 1e20},
	}, got)

	_, err = convertMapJSONNumbers(map[string]interface{}{"huge": json.Number("1e400")})
	assert.ErrorContains(t, err, `"huge"`)
	assert.ErrorContains(t, err, "1e400")

	_, err = convertMapJSONNumbers(map[string]interface{}{"nested": map[string]interface{}{"huge": json.Number("1e400")}})
	assert.ErrorContains(t, err, `"nested"."huge"`)

	_, err = handleUpdateCustomFields(models.CustomFieldsInput{Partial: map[string]interface{}{"huge": json.Number("1e400")}})
	assert.ErrorContains(t, err, `custom field "huge"`)
}
