package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleIDList(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		want    []int
		wantErr string
	}{
		{"distinct", []string{"1", "2", "3"}, []int{1, 2, 3}, ""},
		{"empty", nil, []int{}, ""},
		{"repeated", []string{"1", "2", "1"}, nil, "validating ids: duplicate id found: 1"},
		{"repeated as written differently", []string{"1", "01"}, nil, "validating ids: duplicate id found: 01"},
		{"repeated with a sign", []string{"+2", "2"}, nil, "validating ids: duplicate id found: 2"},
		{"repeated not a number", []string{"x", "x"}, nil, "validating ids: duplicate id found: x"},
		{"not a number", []string{"1", "x"}, nil, "converting ids:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := handleIDList(tt.ids, "ids")
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
