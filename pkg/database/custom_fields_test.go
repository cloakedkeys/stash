//go:build integration
// +build integration

package database_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type customFieldsReaderWriter interface {
	models.CustomFieldsReader
	models.CustomFieldsWriter
}

func testSetCustomFields(t *testing.T, namePrefix string, store customFieldsReaderWriter, id int, origCustomFields map[string]interface{}) {
	getCustomFields := func() map[string]interface{} {
		m := make(map[string]interface{})
		for k, v := range origCustomFields {
			m[k] = v
		}
		return m
	}

	mergeCustomFields := func(i map[string]interface{}) map[string]interface{} {
		m := getCustomFields()

		for k, v := range i {
			m[k] = v
		}
		return m
	}

	tests := []struct {
		name     string
		input    models.CustomFieldsInput
		expected map[string]interface{}
		wantErr  bool
	}{
		{
			"valid full",
			models.CustomFieldsInput{
				Full: map[string]interface{}{
					"key": "value",
				},
			},
			map[string]interface{}{
				"key": "value",
			},
			false,
		},
		{
			"valid partial",
			models.CustomFieldsInput{
				Partial: map[string]interface{}{
					"key": "value",
				},
			},
			mergeCustomFields(map[string]interface{}{
				"key": "value",
			}),
			false,
		},
		{
			"valid partial overwrite",
			models.CustomFieldsInput{
				Partial: map[string]interface{}{
					"real": float64(4.56),
				},
			},
			mergeCustomFields(map[string]interface{}{
				"real": float64(4.56),
			}),
			false,
		},
		{
			"valid remove",
			models.CustomFieldsInput{
				Remove: []string{"real"},
			},
			func() map[string]interface{} {
				m := getCustomFields()
				delete(m, "real")
				return m
			}(),
			false,
		},
		{
			"leading space full",
			models.CustomFieldsInput{
				Full: map[string]interface{}{
					" key": "value",
				},
			},
			nil,
			true,
		},
		{
			"trailing space full",
			models.CustomFieldsInput{
				Full: map[string]interface{}{
					"key ": "value",
				},
			},
			nil,
			true,
		},
		{
			"leading space partial",
			models.CustomFieldsInput{
				Partial: map[string]interface{}{
					" key": "value",
				},
			},
			nil,
			true,
		},
		{
			"trailing space partial",
			models.CustomFieldsInput{
				Partial: map[string]interface{}{
					"key ": "value",
				},
			},
			nil,
			true,
		},
		{
			"big key full",
			models.CustomFieldsInput{
				Full: map[string]interface{}{
					"12345678901234567890123456789012345678901234567890123456789012345": "value",
				},
			},
			nil,
			true,
		},
		{
			"big key partial",
			models.CustomFieldsInput{
				Partial: map[string]interface{}{
					"12345678901234567890123456789012345678901234567890123456789012345": "value",
				},
			},
			nil,
			true,
		},
		{
			"empty key full",
			models.CustomFieldsInput{
				Full: map[string]interface{}{
					"": "value",
				},
			},
			nil,
			true,
		},
		{
			"empty key partial",
			models.CustomFieldsInput{
				Partial: map[string]interface{}{
					"": "value",
				},
			},
			nil,
			true,
		},
		{
			"invalid remove full",
			models.CustomFieldsInput{
				Full: map[string]interface{}{
					"key": "value",
				},
				Remove: []string{"key"},
			},
			nil,
			true,
		},
		{
			"invalid remove partial",
			models.CustomFieldsInput{
				Partial: map[string]interface{}{
					"real": float64(4.56),
				},
				Remove: []string{"real"},
			},
			nil,
			true,
		},
	}

	for _, tt := range tests {
		runWithRollbackTxn(t, namePrefix+" "+tt.name, func(t *testing.T, ctx context.Context) {
			assert := assert.New(t)

			err := store.SetCustomFields(ctx, id, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetCustomFields() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			actual, err := store.GetCustomFields(ctx, id)
			if err != nil {
				t.Errorf("GetCustomFields() error = %v", err)
				return
			}

			assert.Equal(tt.expected, actual)
		})
	}
}

func TestPerformerSetCustomFields(t *testing.T) {
	performerIdx := performerIdx1WithScene

	testSetCustomFields(t, "Performer", db.Performer(), performerIDs[performerIdx], getPerformerCustomFields(performerIdx))
}

func TestTagSetCustomFields(t *testing.T) {
	tagIdx := tagIdx1WithScene

	testSetCustomFields(t, "Tag", db.Tag(), tagIDs[tagIdx], getTagCustomFields(tagIdx))
}

func TestStudioSetCustomFields(t *testing.T) {
	studioIdx := studioIdxWithScene

	testSetCustomFields(t, "Studio", db.Studio(), studioIDs[studioIdx], getStudioCustomFields(studioIdx))
}

func TestSceneSetCustomFields(t *testing.T) {
	sceneIdx := sceneIdxWithPerformer

	testSetCustomFields(t, "Scene", db.Scene(), sceneIDs[sceneIdx], getSceneCustomFields(sceneIdx))
}

func TestGallerySetCustomFields(t *testing.T) {
	galleryIdx := galleryIdxWithChapters

	testSetCustomFields(t, "Gallery", db.Gallery(), galleryIDs[galleryIdx], getGalleryCustomFields(galleryIdx))
}

func TestImageSetCustomFields(t *testing.T) {
	imageIdx := imageIdx2WithGallery

	testSetCustomFields(t, "Image", db.Image(), imageIDs[imageIdx], getImageCustomFields(imageIdx))
}

// A custom field value reads as the same text on both databases, so includes,
// excludes and regex filters find the same items (#2056). Both read a value as
// SQLite always has.
func TestCustomFieldValueText(t *testing.T) {
	performerIdx := performerIdx1WithScene
	fields := map[string]interface{}{
		"text_one":   float64(1.0),
		"text_big":   float64(1e20),
		"text_small": float64(0.00001),
		"text_true":  true,
		"text_false": false,
		"text_neg":   float64(-2),
		"text_int":   int64(5),
	}

	tests := []struct {
		field    string
		modifier models.CriterionModifier
		value    interface{}
		found    bool
	}{
		{"text_one", models.CriterionModifierMatchesRegex, `^1\.0$`, true},
		{"text_one", models.CriterionModifierIncludes, "1.0", true},
		{"text_one", models.CriterionModifierExcludes, "1.0", false},
		{"text_one", models.CriterionModifierNotMatchesRegex, `^1\.0$`, false},
		{"text_big", models.CriterionModifierMatchesRegex, `^1\.0e\+20$`, true},
		{"text_big", models.CriterionModifierIncludes, "100000", false},
		{"text_small", models.CriterionModifierMatchesRegex, `^1\.0e-05$`, true},
		{"text_small", models.CriterionModifierIncludes, "0.00001", false},
		{"text_true", models.CriterionModifierMatchesRegex, `^1$`, true},
		{"text_true", models.CriterionModifierIncludes, "true", false},
		{"text_true", models.CriterionModifierExcludes, "true", true},
		{"text_false", models.CriterionModifierMatchesRegex, `^0$`, true},
		{"text_neg", models.CriterionModifierMatchesRegex, `^-2\.0$`, true},
		{"text_int", models.CriterionModifierMatchesRegex, `^5$`, true},
		// a number searched for as SQLite's store writes it: 1.0 is 1
		{"text_int", models.CriterionModifierIncludes, json.Number("5.0"), true},
		{"text_one", models.CriterionModifierIncludes, json.Number("1.0"), true},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s %s %T %v", tt.field, tt.modifier, tt.value, tt.value)
		runWithRollbackTxn(t, name, func(t *testing.T, ctx context.Context) {
			id := performerIDs[performerIdx]
			require.NoError(t, db.Performer().SetCustomFields(ctx, id, models.CustomFieldsInput{Partial: fields}))

			performers, _, err := db.Performer().Query(ctx, &models.PerformerFilterType{
				Name: &models.StringCriterionInput{
					Value:    getPerformerStringValue(performerIdx, "Name"),
					Modifier: models.CriterionModifierEquals,
				},
				CustomFields: []models.CustomFieldCriterionInput{
					{Field: tt.field, Modifier: tt.modifier, Value: []any{tt.value}},
				},
			}, nil)
			require.NoError(t, err)

			if tt.found {
				assert.Contains(t, performersToIDs(performers), id)
			} else {
				assert.NotContains(t, performersToIDs(performers), id)
			}
		})
	}
}

func TestGroupSetCustomFields(t *testing.T) {
	groupIdx := groupIdxWithScene

	testSetCustomFields(t, "Group", db.Group(), groupIDs[groupIdx], getGroupCustomFields(groupIdx))
}
