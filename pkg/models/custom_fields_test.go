package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// #2061: whole numbers that fit stay int64, everything else that a float64
// holds becomes one, and a number too large even for that is an error.
func TestJSONNumberValue(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want interface{}
	}{
		{"5", int64(5)},
		{"-12", int64(-12)},
		{"0", int64(0)},
		{"9223372036854775807", int64(9223372036854775807)},
		{"1.5", 1.5},
		{"5.0", 5.0},
		{"1e20", 1e20},
		{"1E5", 1e5},
		{"-1e3", -1e3},
		{"99999999999999999999", 1e20},
		{"-99999999999999999999", -1e20},
	} {
		got, err := JSONNumberValue(json.Number(tt.in))
		assert.NoError(t, err, tt.in)
		assert.Equal(t, tt.want, got, tt.in)
	}

	for _, in := range []string{"1e400", "-1e400", "abc"} {
		_, err := JSONNumberValue(json.Number(in))
		assert.ErrorContains(t, err, in)
	}
}

func TestCustomFieldRegexPattern(t *testing.T) {
	for _, tt := range []struct {
		in   interface{}
		want string
	}{
		{"^a.*$", "^a.*$"},
		{json.Number("5"), "5"},
		{json.Number("5.0"), "5"},
		{json.Number("1.5"), "1.5"},
		{json.Number("-12"), "-12"},
		{json.Number("1e20"), "100000000000000000000"},
		{5, "5"},
		{int64(5), "5"},
		{float64(5), "5"},
		{1.25, "1.25"},
		{0.1, "0.1"},
	} {
		got, err := CustomFieldRegexPattern(tt.in)
		assert.NoError(t, err, "%#v", tt.in)
		assert.Equal(t, tt.want, got, "%#v", tt.in)
	}

	for _, in := range []interface{}{true, []interface{}{"a"}, map[string]interface{}{}, nil} {
		_, err := CustomFieldRegexPattern(in)
		assert.Error(t, err, "%#v", in)
	}
}
