package postgres

import (
	"encoding/json"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

// #2062: a regex value that is not text or a number stops the filter, with
// no query built from an empty pattern.
func TestCustomFieldRegexRejectsNonText(t *testing.T) {
	for _, modifier := range []models.CriterionModifier{models.CriterionModifierMatchesRegex, models.CriterionModifierNotMatchesRegex} {
		for _, v := range []any{true, []any{"a"}, nil} {
			h := &customFieldsFilterHandler{table: "performer_custom_fields", fkCol: "performer_id", idCol: "performers.id"}
			f := &filterBuilder{}
			h.handleCriterion(f, "cf", models.CustomFieldCriterionInput{Field: "f", Modifier: modifier, Value: []any{v}})
			assert.Error(t, f.getError(), "%s %v", modifier, v)
			assert.Empty(t, f.whereClauses, "%s %v", modifier, v)
		}
	}
}

func TestCustomFieldRegexNumberPattern(t *testing.T) {
	h := &customFieldsFilterHandler{table: "performer_custom_fields", fkCol: "performer_id", idCol: "performers.id"}
	f := &filterBuilder{}
	h.handleCriterion(f, "cf", models.CustomFieldCriterionInput{Field: "f", Modifier: models.CriterionModifierMatchesRegex, Value: []any{json.Number("5")}})
	assert.NoError(t, f.getError())
	_, args := f.generateWhereClauses()
	assert.Equal(t, []interface{}{"5"}, args)
}
