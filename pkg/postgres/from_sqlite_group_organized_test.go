package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupOrganizedCopyPlan(t *testing.T) {
	plan, err := planTable("groups", []copyColumn{
		{name: "id", dataType: "integer"},
		{name: "organized", dataType: "boolean"},
	}, map[string]bool{"id": true, "organized": true})
	require.NoError(t, err)

	for _, tt := range []struct {
		name  string
		input any
		want  bool
	}{
		{"integer false", int64(0), false},
		{"integer true", int64(1), true},
		{"boolean false", false, false},
		{"boolean true", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			row, problems := plan.convertRow([]any{int64(11), tt.input})
			require.Empty(t, problems)
			assert.Equal(t, []any{int64(11), tt.want}, row)
		})
	}

	_, err = planTable("groups", []copyColumn{
		{name: "id", dataType: "integer"},
		{name: "organized", dataType: "boolean"},
	}, map[string]bool{"id": true})
	assert.Error(t, err, "an older SQLite schema must be upgraded before copying")
}
