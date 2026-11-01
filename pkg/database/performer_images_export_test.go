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
		require.NoError(t, qb.UpdateImage(ctx, source, []byte("primary portrait")))
		a := addPortrait(t, ctx, source, "alternate a")
		b := addPortrait(t, ctx, source, "alternate b")
		require.NoError(t, qb.ReorderAlternateImages(ctx, source, []string{b, a}))
		p, err := qb.Find(ctx, source)
		require.NoError(t, err)
		exported, err := performer.ToJSON(ctx, qb, p)
		require.NoError(t, err)
		require.Equal(t, utils.GetBase64StringFromData([]byte("primary portrait")), exported.Image)
		require.Equal(t, []string{utils.GetBase64StringFromData([]byte("alternate b")), utils.GetBase64StringFromData([]byte("alternate a"))}, exported.AlternateImages)
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
			require.Equal(t, []byte("primary portrait"), data)
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
		require.NoError(t, json.Unmarshal([]byte(`{"name":"legacy","image":"bGVnYWN5IHByaW1hcnk="}`), &old))
		require.Nil(t, old.AlternateImages)
		importer = &performer.Importer{ReaderWriter: qb, TagWriter: db.Tag(), Input: old}
		require.NoError(t, importer.PreImport(ctx))
		require.NoError(t, importer.Update(ctx, *id))
		require.NoError(t, importer.PostImport(ctx, *id))
		require.Equal(t, []string{b, a}, portraitChecksums(t, ctx, *id))
		data, err := qb.GetImage(ctx, *id)
		require.NoError(t, err)
		require.Equal(t, []byte("legacy primary"), data)
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
