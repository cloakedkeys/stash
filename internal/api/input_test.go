package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleIDList(t *testing.T) {
	tests := []struct {
		name     string
		ids      []string
		want     []int
		wantErr  string
		findWant []int
		// error from handleFindIDList, if different from wantErr
		findErr string
	}{
		{"distinct", []string{"1", "2", "3"}, []int{1, 2, 3}, "", []int{1, 2, 3}, ""},
		{"empty", nil, []int{}, "", []int{}, ""},
		{"repeated", []string{"1", "2", "1"}, nil, "validating ids: duplicate id found: 1", nil, ""},
		{"repeated not a number", []string{"x", "x"}, nil, "validating ids: duplicate id found: x", nil, ""},
		{"not a number", []string{"1", "x"}, nil, "converting ids:", nil, ""},
		// only find queries compare by value: other lists, such as voice
		// actor credits, accepted these and still do
		{"repeated as written differently", []string{"1", "01"}, []int{1, 1}, "", nil, "validating ids: duplicate id found: 1"},
		{"repeated with a sign", []string{"+2", "2"}, []int{2, 2}, "", nil, "validating ids: duplicate id found: 2"},
	}

	check := func(t *testing.T, got []int, err error, want []int, wantErr string) {
		t.Helper()
		if wantErr != "" {
			assert.ErrorContains(t, err, wantErr)
			assert.Nil(t, got)
			return
		}
		assert.NoError(t, err)
		assert.Equal(t, want, got)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := handleIDList(tt.ids, "ids")
			check(t, got, err, tt.want, tt.wantErr)

			findErr := tt.findErr
			if findErr == "" {
				findErr = tt.wantErr
			}
			got, err = handleFindIDList(tt.ids, "ids")
			check(t, got, err, tt.findWant, findErr)
		})
	}
}
