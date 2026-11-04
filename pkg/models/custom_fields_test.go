package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
