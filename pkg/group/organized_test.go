package group

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stashapp/stash/pkg/models/mocks"
)

func TestOrganizedExportImport(t *testing.T) {
	for _, value := range []bool{false, true} {
		name := "not organized"
		if value {
			name = "organized"
		}
		t.Run(name, func(t *testing.T) {
			db := mocks.NewDatabase()
			original := models.NewGroup()
			original.ID = 1
			original.Name = name
			original.Organized = value
			original.Aliases = models.NewRelatedStrings([]string{})
			original.URLs = models.NewRelatedStrings([]string{})

			db.Group.On("GetFrontImage", testCtx, original.ID).Return(nil, nil).Once()
			db.Group.On("GetBackImage", testCtx, original.ID).Return(nil, nil).Once()
			db.Group.On("GetCustomFields", testCtx, original.ID).Return(emptyCustomFields, nil).Once()
			exported, err := ToJSON(testCtx, db.Group, db.Studio, &original)
			require.NoError(t, err)
			assert.Equal(t, value, exported.Organized)

			file := filepath.Join(t.TempDir(), "group.json")
			require.NoError(t, jsonschema.SaveGroupFile(file, exported))
			loaded, err := jsonschema.LoadGroupFile(file)
			require.NoError(t, err)
			importer := Importer{ReaderWriter: db.Group, Input: *loaded}
			require.NoError(t, importer.PreImport(testCtx))
			assert.Equal(t, value, importer.group.Organized)

			db.Group.On("Create", testCtx, mock.MatchedBy(func(g *models.Group) bool {
				return g.Name == name && g.Organized == value
			})).Run(func(args mock.Arguments) {
				args.Get(1).(*models.Group).ID = 2
			}).Return(nil).Once()
			id, err := importer.Create(testCtx)
			require.NoError(t, err)
			assert.Equal(t, 2, *id)

			db.Group.On("Update", testCtx, mock.MatchedBy(func(g *models.Group) bool {
				return g.ID == original.ID && g.Organized == value
			})).Return(nil).Once()
			require.NoError(t, importer.Update(testCtx, original.ID))
			db.AssertExpectations(t)
		})
	}
}

func TestImportWithoutOrganized(t *testing.T) {
	importer := Importer{Input: jsonschema.Group{Name: "legacy group"}}
	require.NoError(t, importer.PreImport(testCtx))
	assert.False(t, importer.group.Organized)
}
