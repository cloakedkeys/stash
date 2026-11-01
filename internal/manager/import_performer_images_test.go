package manager

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/performer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDuplicatePerformerImportImages(t *testing.T) {
	for _, behavior := range AllImportDuplicateEnum {
		t.Run(string(behavior), func(t *testing.T) {
			db := mocks.NewDatabase()
			primary := []byte("imported primary")
			alternate := []byte("imported alternate")
			input := jsonschema.Performer{Name: "existing", Image: base64.StdEncoding.EncodeToString(primary), AlternateImages: []string{base64.StdEncoding.EncodeToString(alternate), base64.StdEncoding.EncodeToString(alternate)}}
			db.Performer.On("Query", mock.Anything, mock.Anything, mock.Anything).Return([]*models.Performer{{ID: 7, Name: "existing"}}, 1, nil).Once()
			if behavior == ImportDuplicateEnumOverwrite {
				db.Performer.On("Update", mock.Anything, mock.Anything).Return(nil).Once()
				db.Performer.On("UpdateImage", mock.Anything, 7, primary).Return(nil).Once()
				db.Performer.On("GetAlternateImages", mock.Anything, 7).Return([]models.PerformerImage{{Checksum: "old"}}, nil).Once()
				db.Performer.On("RemoveAlternateImage", mock.Anything, 7, "old").Return(nil).Once()
				db.Performer.On("AddAlternateImage", mock.Anything, 7, alternate).Return(true, nil).Once()
				db.Performer.On("AddAlternateImage", mock.Anything, 7, alternate).Return(false, nil).Once()
			}
			i := &performer.Importer{ReaderWriter: db.Performer, TagWriter: db.Tag, Input: input}
			err := performImport(context.Background(), i, behavior)
			if behavior == ImportDuplicateEnumFail {
				require.ErrorContains(t, err, "existing object")
			} else {
				require.NoError(t, err)
			}
			// IGNORE and FAIL must not call any image writer or PostImport.
			db.AssertExpectations(t)
		})
	}
}
