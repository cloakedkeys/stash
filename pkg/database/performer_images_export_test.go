//go:build integration

package database_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stashapp/stash/pkg/performer"
	"github.com/stashapp/stash/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestPerformerImagesExportImportRoundTrip(t *testing.T) {
	runWithRollbackTxn(t, "ordered images and duplicate overwrite", func(t *testing.T, ctx context.Context) {
		qb := db.Performer()
		source := createPortraitPerformer(t, ctx, t.Name())
		require.NoError(t, qb.UpdateImage(ctx, source, []byte("<svg><title>primary portrait</title></svg>")))
		a := addPortrait(t, ctx, source, "<svg><title>alternate a</title></svg>")
		b := addPortrait(t, ctx, source, "<svg><title>alternate b</title></svg>")
		require.NoError(t, qb.ReorderAlternateImages(ctx, source, []string{b, a}))
		p, err := qb.Find(ctx, source)
		require.NoError(t, err)
		exported, err := performer.ToJSON(ctx, qb, p)
		require.NoError(t, err)
		require.Equal(t, utils.GetBase64StringFromData([]byte("<svg><title>primary portrait</title></svg>")), exported.Image)
		require.Equal(t, []string{utils.GetBase64StringFromData([]byte("<svg><title>alternate b</title></svg>")), utils.GetBase64StringFromData([]byte("<svg><title>alternate a</title></svg>"))}, exported.AlternateImages)
		encoded, err := json.Marshal(exported)
		require.NoError(t, err)
		var input jsonschema.Performer
		require.NoError(t, json.Unmarshal(encoded, &input))
		input.Name += " imported"
		// Duplicate bytes and the primary in the alternate list must not create extra references.
		input.AlternateImages = append(input.AlternateImages, input.AlternateImages[0], input.Image)
		importer := &performer.Importer{ReaderWriter: qb, TagWriter: db.Tag(), Input: input}
		require.NoError(t, importer.PreImport(ctx))
		id, err := importer.Create(ctx)
		require.NoError(t, err)
		require.NoError(t, importer.PostImport(ctx, *id))
		check := func() {
			data, err := qb.GetImage(ctx, *id)
			require.NoError(t, err)
			require.Equal(t, []byte("<svg><title>primary portrait</title></svg>"), data)
			require.Equal(t, []string{b, a}, portraitChecksums(t, ctx, *id))
		}
		check()
		// Overwrite replaces a changed list and is stable on repeated imports.
		addPortrait(t, ctx, *id, "obsolete alternate")
		require.NoError(t, qb.ReorderAlternateImages(ctx, *id, []string{a, b, portraitChecksums(t, ctx, *id)[2]}))
		for n := 0; n < 2; n++ {
			importer = &performer.Importer{ReaderWriter: qb, TagWriter: db.Tag(), Input: input}
			require.NoError(t, importer.PreImport(ctx))
			existing, err := importer.FindExistingID(ctx)
			require.NoError(t, err)
			require.Equal(t, id, existing)
			require.NoError(t, importer.Update(ctx, *existing))
			require.NoError(t, importer.PostImport(ctx, *existing))
			check()
		}
		// A real old-format JSON has no alternate list. Its primary update preserves alternates.
		var old jsonschema.Performer
		require.NoError(t, json.Unmarshal([]byte(`{"name":"legacy","image":"PHN2Zz48dGl0bGU+bGVnYWN5IHByaW1hcnk8L3RpdGxlPjwvc3ZnPg=="}`), &old))
		require.Nil(t, old.AlternateImages)
		importer = &performer.Importer{ReaderWriter: qb, TagWriter: db.Tag(), Input: old}
		require.NoError(t, importer.PreImport(ctx))
		require.NoError(t, importer.Update(ctx, *id))
		require.NoError(t, importer.PostImport(ctx, *id))
		require.Equal(t, []string{b, a}, portraitChecksums(t, ctx, *id))
		data, err := qb.GetImage(ctx, *id)
		require.NoError(t, err)
		require.Equal(t, []byte("<svg><title>legacy primary</title></svg>"), data)
		old.Name = "legacy new"
		importer = &performer.Importer{ReaderWriter: qb, TagWriter: db.Tag(), Input: old}
		require.NoError(t, importer.PreImport(ctx))
		legacyID, err := importer.Create(ctx)
		require.NoError(t, err)
		require.NoError(t, importer.PostImport(ctx, *legacyID))
		require.Empty(t, portraitChecksums(t, ctx, *legacyID))
		// An explicit empty new-format list clears the list, and exports back as [].
		input.AlternateImages = []string{}
		importer = &performer.Importer{ReaderWriter: qb, TagWriter: db.Tag(), Input: input}
		require.NoError(t, importer.PreImport(ctx))
		require.NoError(t, importer.PostImport(ctx, *id))
		require.Empty(t, portraitChecksums(t, ctx, *id))
		p, err = qb.Find(ctx, *id)
		require.NoError(t, err)
		exported, err = performer.ToJSON(ctx, qb, p)
		require.NoError(t, err)
		encoded, err = json.Marshal(exported)
		require.NoError(t, err)
		require.Contains(t, string(encoded), `"alternate_images":[]`)
	})
}

func TestPerformerAlternateImagesBulk(t *testing.T) {
	runWithRollbackTxn(t, "ordered metadata aligned to IDs", func(t *testing.T, ctx context.Context) {
		a := createPortraitPerformer(t, ctx, t.Name()+" a")
		b := createPortraitPerformer(t, ctx, t.Name()+" b")
		c := createPortraitPerformer(t, ctx, t.Name()+" c")
		a1 := addPortrait(t, ctx, a, "a1")
		a2 := addPortrait(t, ctx, a, "a2")
		b1 := addPortrait(t, ctx, b, "b1")
		require.NoError(t, db.Performer().ReorderAlternateImages(ctx, a, []string{a2, a1}))
		rows, err := db.Performer().GetAlternateImagesBulk(ctx, []int{c, b, a, b})
		require.NoError(t, err)
		require.Len(t, rows, 4)
		require.Empty(t, rows[0])
		require.Equal(t, b1, rows[1][0].Checksum)
		require.Equal(t, a2, rows[2][0].Checksum)
		require.Equal(t, a1, rows[2][1].Checksum)
		require.Equal(t, rows[1], rows[3])
		rows, err = db.Performer().GetAlternateImagesBulk(ctx, nil)
		require.NoError(t, err)
		require.Empty(t, rows)
	})
}

func TestPerformerImportSkipsLegacyTextImages(t *testing.T) {
	runWithRollbackTxn(t, "legacy text does not lose metadata", func(t *testing.T, ctx context.Context) {
		qb := db.Performer()
		svg := []byte(`<svg/>`)
		input := jsonschema.Performer{
			Name:    t.Name(),
			Details: "Restored metadata",
			Image:   utils.GetBase64StringFromData([]byte("broken primary image")),
			AlternateImages: []string{
				utils.GetBase64StringFromData([]byte("broken alternate image")),
				utils.GetBase64StringFromData(svg),
			},
		}
		i := &performer.Importer{ReaderWriter: qb, Input: input}
		require.NoError(t, i.PreImport(ctx))
		id, err := i.Create(ctx)
		require.NoError(t, err)
		require.NoError(t, i.PostImport(ctx, *id))
		p, err := qb.Find(ctx, *id)
		require.NoError(t, err)
		require.Equal(t, input.Details, p.Details)
		image, err := qb.GetImage(ctx, *id)
		require.NoError(t, err)
		require.Empty(t, image)
		images, err := qb.GetAlternateImages(ctx, *id)
		require.NoError(t, err)
		require.Len(t, images, 1)

		// Full metadata overwrite clears the primary in the existing store update;
		// skipping the invalid replacement must still retain the imported metadata.
		require.NoError(t, qb.UpdateImage(ctx, *id, svg))
		input.Details = "Updated metadata"
		input.AlternateImages = nil
		i = &performer.Importer{ReaderWriter: qb, Input: input}
		require.NoError(t, i.PreImport(ctx))
		require.NoError(t, i.Update(ctx, *id))
		require.NoError(t, i.PostImport(ctx, *id))
		p, err = qb.Find(ctx, *id)
		require.NoError(t, err)
		require.Equal(t, input.Details, p.Details)
		image, err = qb.GetImage(ctx, *id)
		require.NoError(t, err)
		require.Empty(t, image)
	})
}
