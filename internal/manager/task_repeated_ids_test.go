package manager

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Export and generate handle each item once, even if its id is listed more
// than once or written differently (#2078).
func TestTaskIDsRepeated(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		want []int
	}{
		{"distinct", []string{"3", "1", "2"}, []int{3, 1, 2}},
		{"repeated", []string{"1", "2", "1", "2"}, []int{1, 2}},
		{"written differently", []string{"1", "01"}, []int{1}},
		{"none", nil, []int{}},
	}

	for _, tt := range tests {
		t.Run("export "+tt.name, func(t *testing.T) {
			spec := newExportSpec(&ExportObjectTypeInput{Ids: tt.ids})
			assert.Equal(t, tt.want, spec.IDs)
		})

		t.Run("generate "+tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, generateIDs(tt.ids))
		})
	}
}
