package tag

import (
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// #2218: an import set the parents without the loop check that tag edits
// use. A parent that is already under the tag would make the tag its own
// ancestor: the import leaves out only that parent.
func TestImporterPostImportParentLoop(t *testing.T) {
	const (
		childID  = 1
		parentID = 2 // already under Child
		otherID  = 3
	)
	db := mocks.NewDatabase()

	i := Importer{
		ReaderWriter: db.Tag,
		Input: jsonschema.Tag{
			Name:    "Child",
			Parents: []string{"Parent", "Other", "Child"},
		},
	}

	db.Tag.On("UpdateAliases", testCtx, childID, mock.Anything).Return(nil).Once()
	db.Tag.On("FindByName", testCtx, "Parent", false).Return(&models.Tag{ID: parentID, Name: "Parent"}, nil).Once()
	db.Tag.On("FindByName", testCtx, "Other", false).Return(&models.Tag{ID: otherID, Name: "Other"}, nil).Once()
	db.Tag.On("FindByName", testCtx, "Child", false).Return(&models.Tag{ID: childID, Name: "Child"}, nil).Once()
	db.Tag.On("FindAllAncestors", testCtx, childID, mock.Anything).Return([]*models.TagPath{
		{Tag: models.Tag{ID: childID, Name: "Child"}, Path: "Child"},
	}, nil)
	db.Tag.On("FindAllDescendants", testCtx, childID, mock.Anything).Return([]*models.TagPath{
		{Tag: models.Tag{ID: childID, Name: "Child"}, Path: "Child"},
		{Tag: models.Tag{ID: parentID, Name: "Parent"}, Path: "Child->Parent"},
	}, nil)
	// Parent is under Child, and Child can't be its own parent: only Other is set
	db.Tag.On("UpdateParentTags", testCtx, childID, []int{otherID}).Return(nil).Once()

	err := i.PostImport(testCtx, childID)
	assert.NoError(t, err)
	db.AssertExpectations(t)
}
