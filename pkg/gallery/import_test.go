package gallery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/json"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	existingStudioID    = 101
	existingPerformerID = 103
	existingTagID       = 105

	existingStudioName = "existingStudioName"
	existingStudioErr  = "existingStudioErr"
	missingStudioName  = "missingStudioName"

	existingPerformerName = "existingPerformerName"
	existingPerformerErr  = "existingPerformerErr"
	missingPerformerName  = "missingPerformerName"

	existingTagName = "existingTagName"
	existingTagErr  = "existingTagErr"
	missingTagName  = "missingTagName"
)

var testCtx = context.Background()

var (
	createdAt = time.Date(2001, time.January, 2, 1, 2, 3, 4, time.Local)
	updatedAt = time.Date(2002, time.January, 2, 1, 2, 3, 4, time.Local)
)

func TestImporterPreImport(t *testing.T) {
	i := Importer{
		Input: jsonschema.Gallery{
			Title:     title,
			Date:      date,
			Details:   details,
			Rating:    rating,
			Organized: organized,
			OCounter:  ocounter,
			URL:       url,
			CreatedAt: json.JSONTime{
				Time: createdAt,
			},
			UpdatedAt: json.JSONTime{
				Time: updatedAt,
			},
		},
	}

	err := i.PreImport(testCtx)
	assert.Nil(t, err)

	expectedGallery := models.Gallery{
		Title:        title,
		Date:         &dateObj,
		Details:      details,
		Rating:       &rating,
		Organized:    organized,
		OCounter:     ocounter,
		URLs:         models.NewRelatedStrings([]string{url}),
		Files:        models.NewRelatedFiles([]models.File{}),
		TagIDs:       models.NewRelatedIDs([]int{}),
		PerformerIDs: models.NewRelatedIDs([]int{}),
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}

	assert.Equal(t, expectedGallery, i.gallery)
}

func TestImporterPreImportWithStudio(t *testing.T) {
	db := mocks.NewDatabase()

	i := Importer{
		StudioWriter: db.Studio,
		Input: jsonschema.Gallery{
			Studio: existingStudioName,
		},
	}

	db.Studio.On("FindByName", testCtx, existingStudioName, false).Return(&models.Studio{
		ID: existingStudioID,
	}, nil).Once()
	db.Studio.On("FindByName", testCtx, existingStudioErr, false).Return(nil, errors.New("FindByName error")).Once()

	err := i.PreImport(testCtx)
	assert.Nil(t, err)
	assert.Equal(t, existingStudioID, *i.gallery.StudioID)

	i.Input.Studio = existingStudioErr
	err = i.PreImport(testCtx)
	assert.NotNil(t, err)

	db.AssertExpectations(t)
}

func TestImporterPreImportWithMissingStudio(t *testing.T) {
	db := mocks.NewDatabase()

	i := Importer{
		StudioWriter: db.Studio,
		Input: jsonschema.Gallery{
			Studio: missingStudioName,
		},
		MissingRefBehaviour: models.ImportMissingRefEnumFail,
	}

	db.Studio.On("FindByName", testCtx, missingStudioName, false).Return(nil, nil).Times(3)
	db.Studio.On("Create", testCtx, mock.AnythingOfType("*models.CreateStudioInput")).Run(func(args mock.Arguments) {
		s := args.Get(1).(*models.CreateStudioInput)
		s.Studio.ID = existingStudioID
	}).Return(nil)

	err := i.PreImport(testCtx)
	assert.NotNil(t, err)

	i.MissingRefBehaviour = models.ImportMissingRefEnumIgnore
	err = i.PreImport(testCtx)
	assert.Nil(t, err)

	i.MissingRefBehaviour = models.ImportMissingRefEnumCreate
	err = i.PreImport(testCtx)
	assert.Nil(t, err)
	assert.Equal(t, existingStudioID, *i.gallery.StudioID)

	db.AssertExpectations(t)
}

func TestImporterPreImportWithMissingStudioCreateErr(t *testing.T) {
	db := mocks.NewDatabase()

	i := Importer{
		StudioWriter: db.Studio,
		Input: jsonschema.Gallery{
			Studio: missingStudioName,
		},
		MissingRefBehaviour: models.ImportMissingRefEnumCreate,
	}

	db.Studio.On("FindByName", testCtx, missingStudioName, false).Return(nil, nil).Once()
	db.Studio.On("Create", testCtx, mock.AnythingOfType("*models.CreateStudioInput")).Return(errors.New("Create error"))

	err := i.PreImport(testCtx)
	assert.NotNil(t, err)

	db.AssertExpectations(t)
}

func TestImporterPreImportWithPerformer(t *testing.T) {
	db := mocks.NewDatabase()

	i := Importer{
		PerformerWriter:     db.Performer,
		MissingRefBehaviour: models.ImportMissingRefEnumFail,
		Input: jsonschema.Gallery{
			Performers: []string{
				existingPerformerName,
			},
		},
	}

	db.Performer.On("FindByNames", testCtx, []string{existingPerformerName}, false).Return([]*models.Performer{
		{
			ID:   existingPerformerID,
			Name: existingPerformerName,
		},
	}, nil).Once()
	db.Performer.On("FindByNames", testCtx, []string{existingPerformerErr}, false).Return(nil, errors.New("FindByNames error")).Once()

	err := i.PreImport(testCtx)
	assert.Nil(t, err)
	assert.Equal(t, []int{existingPerformerID}, i.gallery.PerformerIDs.List())

	i.Input.Performers = []string{existingPerformerErr}
	err = i.PreImport(testCtx)
	assert.NotNil(t, err)

	db.AssertExpectations(t)
}

func TestImporterPreImportWithMissingPerformer(t *testing.T) {
	db := mocks.NewDatabase()

	i := Importer{
		PerformerWriter: db.Performer,
		Input: jsonschema.Gallery{
			Performers: []string{
				missingPerformerName,
			},
		},
		MissingRefBehaviour: models.ImportMissingRefEnumFail,
	}

	db.Performer.On("FindByNames", testCtx, []string{missingPerformerName}, false).Return(nil, nil).Times(3)
	db.Performer.On("Create", testCtx, mock.AnythingOfType("*models.CreatePerformerInput")).Run(func(args mock.Arguments) {
		performer := args.Get(1).(*models.CreatePerformerInput)
		performer.ID = existingPerformerID
	}).Return(nil)

	err := i.PreImport(testCtx)
	assert.NotNil(t, err)

	i.MissingRefBehaviour = models.ImportMissingRefEnumIgnore
	err = i.PreImport(testCtx)
	assert.Nil(t, err)

	i.MissingRefBehaviour = models.ImportMissingRefEnumCreate
	err = i.PreImport(testCtx)
	assert.Nil(t, err)
	assert.Equal(t, []int{existingPerformerID}, i.gallery.PerformerIDs.List())

	db.AssertExpectations(t)
}

func TestImporterPreImportWithMissingPerformerCreateErr(t *testing.T) {
	db := mocks.NewDatabase()

	i := Importer{
		PerformerWriter: db.Performer,
		Input: jsonschema.Gallery{
			Performers: []string{
				missingPerformerName,
			},
		},
		MissingRefBehaviour: models.ImportMissingRefEnumCreate,
	}

	db.Performer.On("FindByNames", testCtx, []string{missingPerformerName}, false).Return(nil, nil).Once()
	db.Performer.On("Create", testCtx, mock.AnythingOfType("*models.CreatePerformerInput")).Return(errors.New("Create error"))

	err := i.PreImport(testCtx)
	assert.NotNil(t, err)

	db.AssertExpectations(t)
}

func TestImporterPreImportWithTag(t *testing.T) {
	db := mocks.NewDatabase()

	i := Importer{
		TagWriter:           db.Tag,
		MissingRefBehaviour: models.ImportMissingRefEnumFail,
		Input: jsonschema.Gallery{
			Tags: []string{
				existingTagName,
			},
		},
	}

	db.Tag.On("FindByNames", testCtx, []string{existingTagName}, false).Return([]*models.Tag{
		{
			ID:   existingTagID,
			Name: existingTagName,
		},
	}, nil).Once()
	db.Tag.On("FindByNames", testCtx, []string{existingTagErr}, false).Return(nil, errors.New("FindByNames error")).Once()

	err := i.PreImport(testCtx)
	assert.Nil(t, err)
	assert.Equal(t, []int{existingTagID}, i.gallery.TagIDs.List())

	i.Input.Tags = []string{existingTagErr}
	err = i.PreImport(testCtx)
	assert.NotNil(t, err)

	db.AssertExpectations(t)
}

func TestImporterPreImportWithMissingTag(t *testing.T) {
	db := mocks.NewDatabase()

	i := Importer{
		TagWriter: db.Tag,
		Input: jsonschema.Gallery{
			Tags: []string{
				missingTagName,
			},
		},
		MissingRefBehaviour: models.ImportMissingRefEnumFail,
	}

	db.Tag.On("FindByNames", testCtx, []string{missingTagName}, false).Return(nil, nil).Times(3)
	db.Tag.On("Create", testCtx, mock.AnythingOfType("*models.CreateTagInput")).Run(func(args mock.Arguments) {
		t := args.Get(1).(*models.CreateTagInput)
		t.Tag.ID = existingTagID
	}).Return(nil)

	err := i.PreImport(testCtx)
	assert.NotNil(t, err)

	i.MissingRefBehaviour = models.ImportMissingRefEnumIgnore
	err = i.PreImport(testCtx)
	assert.Nil(t, err)

	i.MissingRefBehaviour = models.ImportMissingRefEnumCreate
	err = i.PreImport(testCtx)
	assert.Nil(t, err)
	assert.Equal(t, []int{existingTagID}, i.gallery.TagIDs.List())

	db.AssertExpectations(t)
}

func TestImporterPreImportWithMissingTagCreateErr(t *testing.T) {
	db := mocks.NewDatabase()

	i := Importer{
		TagWriter: db.Tag,
		Input: jsonschema.Gallery{
			Tags: []string{
				missingTagName,
			},
		},
		MissingRefBehaviour: models.ImportMissingRefEnumCreate,
	}

	db.Tag.On("FindByNames", testCtx, []string{missingTagName}, false).Return(nil, nil).Once()
	db.Tag.On("Create", testCtx, mock.AnythingOfType("*models.CreateTagInput")).Return(errors.New("Create error"))

	err := i.PreImport(testCtx)
	assert.NotNil(t, err)

	db.AssertExpectations(t)
}

func TestImporterFindExistingIDByZipFile(t *testing.T) {
	lookupErr := errors.New("FindByFileID error")
	tests := []struct {
		name    string
		matches [][]*models.Gallery
		err     error
		wantID  int
	}{
		{
			name:    "first file matches and stops lookup",
			matches: [][]*models.Gallery{{{ID: 101}, {ID: 102}}},
			wantID:  101,
		},
		{
			name:    "nil result before match",
			matches: [][]*models.Gallery{nil, {{ID: 101}}},
			wantID:  101,
		},
		{
			name:    "empty result before match",
			matches: [][]*models.Gallery{{}, {{ID: 101}}},
			wantID:  101,
		},
		{
			name:    "no matching files",
			matches: [][]*models.Gallery{nil, {}, nil},
		},
		{
			name:    "first lookup fails",
			matches: [][]*models.Gallery{nil},
			err:     lookupErr,
		},
		{
			name:    "lookup fails after a miss",
			matches: [][]*models.Gallery{{}, nil},
			err:     lookupErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mocks.NewDatabase()
			files := []models.File{
				&models.BaseFile{ID: 1},
				&models.BaseFile{ID: 2},
				&models.BaseFile{ID: 3},
			}
			for n, matches := range tt.matches {
				var err error
				if n == len(tt.matches)-1 {
					err = tt.err
				}
				db.Gallery.On("FindByFileID", testCtx, files[n].Base().ID).Return(matches, err).Once()
			}

			i := Importer{
				ReaderWriter: db.Gallery,
				gallery: models.Gallery{
					Files: models.NewRelatedFiles(files),
				},
			}
			id, err := i.FindExistingID(testCtx)
			assert.Equal(t, tt.err, err)
			if tt.wantID == 0 {
				assert.Nil(t, id)
			} else {
				assert.Equal(t, &tt.wantID, id)
			}
			db.AssertExpectations(t)
		})
	}
}
