package performer

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stashapp/stash/pkg/utils"
	"github.com/stretchr/testify/require"
)

type importImageWriter struct {
	ImporterReaderWriter
	primaryUpdated bool
	alternates     [][]byte
}

func (w *importImageWriter) UpdateImage(context.Context, int, []byte) error {
	w.primaryUpdated = true
	return nil
}

func (w *importImageWriter) GetAlternateImages(context.Context, int) ([]models.PerformerImage, error) {
	return nil, nil
}

func (w *importImageWriter) AddAlternateImage(_ context.Context, _ int, data []byte) (bool, error) {
	w.alternates = append(w.alternates, data)
	return true, nil
}

func TestImportSkipsLegacyTextImages(t *testing.T) {
	ctx := context.Background()
	writer := &importImageWriter{}
	svg := []byte(`<svg/>`)
	i := Importer{
		ReaderWriter: writer,
		Input: jsonschema.Performer{
			Name:    "Restored performer",
			Details: "Keep this metadata",
			Image:   utils.GetBase64StringFromData([]byte("old broken primary")),
			AlternateImages: []string{
				utils.GetBase64StringFromData([]byte("old broken alternate")),
				utils.GetBase64StringFromData(svg),
			},
		},
	}
	require.NoError(t, i.PreImport(ctx))
	require.Equal(t, i.Input.Name, i.performer.Name)
	require.Equal(t, i.Input.Details, i.performer.Details)
	require.NoError(t, i.PostImport(ctx, 1))
	require.False(t, writer.primaryUpdated, "a skipped primary must leave an existing image alone")
	require.Equal(t, [][]byte{svg}, writer.alternates)
}

func TestImportStillRejectsEmptyAlternate(t *testing.T) {
	i := Importer{Input: jsonschema.Performer{AlternateImages: []string{"data:image/png;base64,"}}}
	require.ErrorContains(t, i.PreImport(context.Background()), "alternate image 1 is empty")
}
