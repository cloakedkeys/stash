package sqlite

import (
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

// A custom field modifier given the wrong number of values is refused with an
// error naming that modifier, and never indexes past the values it was given
// (#2067).
func TestCustomFieldCriterionValueCount(t *testing.T) {
	tests := []struct {
		modifier models.CriterionModifier
		values   []interface{}
		wantErr  string
	}{
		{models.CriterionModifierBetween, nil, "expected 2 values for custom field criterion modifier BETWEEN, got 0"},
		{models.CriterionModifierBetween, []interface{}{1}, "expected 2 values for custom field criterion modifier BETWEEN, got 1"},
		{models.CriterionModifierBetween, []interface{}{1, 2}, ""},
		{models.CriterionModifierNotBetween, nil, "expected 2 values for custom field criterion modifier NOT_BETWEEN, got 0"},
		{models.CriterionModifierNotBetween, []interface{}{1}, "expected 2 values for custom field criterion modifier NOT_BETWEEN, got 1"},
		{models.CriterionModifierNotBetween, []interface{}{1, 2, 3}, "expected 2 values for custom field criterion modifier NOT_BETWEEN, got 3"},
		{models.CriterionModifierNotBetween, []interface{}{1, 2}, ""},
		{models.CriterionModifierLessThan, nil, "expected 1 value for custom field criterion modifier LESS_THAN, got 0"},
		{models.CriterionModifierLessThan, []interface{}{1}, ""},
		{models.CriterionModifierGreaterThan, nil, "expected 1 value for custom field criterion modifier GREATER_THAN, got 0"},
		{models.CriterionModifierGreaterThan, []interface{}{1, 2}, "expected 1 value for custom field criterion modifier GREATER_THAN, got 2"},
		{models.CriterionModifierGreaterThan, []interface{}{1}, ""},
	}
	for _, tt := range tests {
		h := &customFieldsFilterHandler{table: "performer_custom_fields", fkCol: "performer_id", idCol: "performers.id"}
		f := &filterBuilder{}
		assert.NotPanics(t, func() {
			h.handleCriterion(f, "cf", models.CustomFieldCriterionInput{Field: "n", Modifier: tt.modifier, Value: tt.values})
		}, "%s with %d values", tt.modifier, len(tt.values))
		err := f.getError()
		if tt.wantErr == "" {
			assert.NoError(t, err, "%s with %d values", tt.modifier, len(tt.values))
			continue
		}
		if assert.Error(t, err, "%s with %d values", tt.modifier, len(tt.values)) {
			assert.Equal(t, tt.wantErr, err.Error())
		}
	}
}
