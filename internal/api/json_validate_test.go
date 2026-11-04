package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// #2084: validateMapJSONNumbers finds a number too large for a float in maps
// and lists at any depth, and leaves every value as it was.
func TestValidateMapJSONNumbers(t *testing.T) {
	input := map[string]interface{}{
		"beyond":  json.Number("99999999999999999999"),
		"decimal": json.Number("1.50"),
		"list":    []interface{}{json.Number("1e20"), map[string]interface{}{"n": json.Number("5")}, "foo", nil},
		"nested":  map[string]interface{}{"list": []interface{}{[]interface{}{json.Number("-1e3")}}},
	}
	want := map[string]interface{}{
		"beyond":  json.Number("99999999999999999999"),
		"decimal": json.Number("1.50"),
		"list":    []interface{}{json.Number("1e20"), map[string]interface{}{"n": json.Number("5")}, "foo", nil},
		"nested":  map[string]interface{}{"list": []interface{}{[]interface{}{json.Number("-1e3")}}},
	}
	assert.NoError(t, validateMapJSONNumbers(input))
	assert.Equal(t, want, input)
	assert.NoError(t, validateMapJSONNumbers(nil))

	for _, tt := range []struct {
		input map[string]interface{}
		want  string
	}{
		{map[string]interface{}{"huge": json.Number("1e400")}, `"huge": invalid number "1e400"`},
		{map[string]interface{}{"a": map[string]interface{}{"b": json.Number("-1e400")}}, `"a"."b": invalid number "-1e400"`},
		{map[string]interface{}{"value": []interface{}{json.Number("1"), json.Number("1e400")}}, `"value"[1]: invalid number "1e400"`},
		{map[string]interface{}{"a": []interface{}{map[string]interface{}{"b": []interface{}{json.Number("1e400")}}}}, `"a"[0]."b"[0]: invalid number "1e400"`},
	} {
		assert.ErrorContains(t, validateMapJSONNumbers(tt.input), tt.want)
	}
}
