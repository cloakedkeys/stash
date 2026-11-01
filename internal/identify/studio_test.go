package identify

import (
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func scrapedStudioWithImage(name string, image string) *models.ScrapedStudio {
	return &models.ScrapedStudio{
		Name:   name,
		Image:  &image,
		Images: []string{image},
	}
}

func mockCreateStudio(db *mocks.Database, name string, id int, err error) {
	db.Studio.On("Create", testCtx, mock.MatchedBy(func(input *models.CreateStudioInput) bool {
		return input.Name == name
	})).Run(func(args mock.Arguments) {
		input := args.Get(1).(*models.CreateStudioInput)
		input.ID = id
	}).Return(err)
}

func Test_createMissingStudio(t *testing.T) {
	emptyEndpoint := ""
	validEndpoint := "validEndpoint"
	invalidEndpoint := "invalidEndpoint"
	remoteSiteID := "remoteSiteID"
	validName := "validName"
	invalidName := "invalidName"
	createdID := 1

	db := mocks.NewDatabase()

	db.Studio.On("Create", testCtx, mock.MatchedBy(func(p *models.CreateStudioInput) bool {
		return p.Name == validName
	})).Run(func(args mock.Arguments) {
		s := args.Get(1).(*models.CreateStudioInput)
		s.ID = createdID
	}).Return(nil)
	db.Studio.On("Create", testCtx, mock.MatchedBy(func(p *models.CreateStudioInput) bool {
		return p.Name == invalidName
	})).Return(errors.New("error creating studio"))

	db.Studio.On("UpdatePartial", testCtx, models.StudioPartial{
		ID: createdID,
		StashIDs: &models.UpdateStashIDs{
			StashIDs: []models.StashID{
				{
					Endpoint: invalidEndpoint,
					StashID:  remoteSiteID,
				},
			},
			Mode: models.RelationshipUpdateModeSet,
		},
	}).Return(nil, errors.New("error updating stash ids"))
	db.Studio.On("UpdatePartial", testCtx, models.StudioPartial{
		ID: createdID,
		StashIDs: &models.UpdateStashIDs{
			StashIDs: []models.StashID{
				{
					Endpoint: validEndpoint,
					StashID:  remoteSiteID,
				},
			},
			Mode: models.RelationshipUpdateModeSet,
		},
	}).Return(models.Studio{
		ID: createdID,
	}, nil)

	type args struct {
		endpoint string
		studio   *models.ScrapedStudio
	}
	tests := []struct {
		name    string
		args    args
		want    *int
		wantErr bool
	}{
		{
			"simple",
			args{
				emptyEndpoint,
				&models.ScrapedStudio{
					Name:         validName,
					RemoteSiteID: &remoteSiteID,
				},
			},
			&createdID,
			false,
		},
		{
			"error creating",
			args{
				emptyEndpoint,
				&models.ScrapedStudio{
					Name:         invalidName,
					RemoteSiteID: &remoteSiteID,
				},
			},
			nil,
			true,
		},
		{
			"valid stash id",
			args{
				validEndpoint,
				&models.ScrapedStudio{
					Name:         validName,
					RemoteSiteID: &remoteSiteID,
				},
			},
			&createdID,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := createMissingStudio(testCtx, tt.args.endpoint, db.Studio, tt.args.studio)
			if (err != nil) != tt.wantErr {
				t.Errorf("createMissingStudio() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createMissingStudio() = %d, want %d", got, tt.want)
			}
		})
	}
}

func Test_createMissingStudio_parentAndImagePaths(t *testing.T) {
	endpoint := "endpoint"
	childName := "Child"
	parentName := "Parent"
	childID := 1
	parentID := 2

	validImage := "data:image/png;base64,AAAA"
	invalidImage := "data:image/png;base64,%%%%"
	storedParentID := strconv.Itoa(parentID)

	tests := []struct {
		name    string
		studio  *models.ScrapedStudio
		setup   func(db *mocks.Database)
		want    *int
		wantErr bool
	}{
		{
			name:   "new parent and child",
			studio: &models.ScrapedStudio{Name: childName, Parent: &models.ScrapedStudio{Name: parentName}},
			setup: func(db *mocks.Database) {
				mockCreateStudio(db, parentName, parentID, nil)
				mockCreateStudio(db, childName, childID, nil)
			},
			want: &childID,
		},
		{
			name:    "new parent image error",
			studio:  &models.ScrapedStudio{Name: childName, Parent: scrapedStudioWithImage(parentName, invalidImage)},
			wantErr: true,
		},
		{
			name:   "new parent create error",
			studio: &models.ScrapedStudio{Name: childName, Parent: &models.ScrapedStudio{Name: parentName}},
			setup: func(db *mocks.Database) {
				mockCreateStudio(db, parentName, parentID, errors.New("error creating parent"))
			},
			wantErr: true,
		},
		{
			name:   "new parent update image error",
			studio: &models.ScrapedStudio{Name: childName, Parent: scrapedStudioWithImage(parentName, validImage)},
			setup: func(db *mocks.Database) {
				mockCreateStudio(db, parentName, parentID, nil)
				db.Studio.On("UpdateImage", testCtx, parentID, mock.AnythingOfType("[]uint8")).Return(errors.New("error updating parent image"))
			},
			wantErr: true,
		},
		{
			name:   "existing parent and child",
			studio: &models.ScrapedStudio{Name: childName, Parent: &models.ScrapedStudio{Name: parentName, StoredID: &storedParentID}},
			setup: func(db *mocks.Database) {
				mockCreateStudio(db, childName, childID, nil)
			},
			want: &childID,
		},
		{
			name: "existing parent ignores invalid scraped image",
			studio: func() *models.ScrapedStudio {
				parent := scrapedStudioWithImage(parentName, invalidImage)
				parent.StoredID = &storedParentID
				return &models.ScrapedStudio{Name: childName, Parent: parent}
			}(),
			setup: func(db *mocks.Database) {
				mockCreateStudio(db, childName, childID, nil)
			},
			want: &childID,
		},
		{
			name:    "main image error",
			studio:  scrapedStudioWithImage(childName, invalidImage),
			wantErr: true,
		},
		{
			name:   "main update image error",
			studio: scrapedStudioWithImage(childName, validImage),
			setup: func(db *mocks.Database) {
				mockCreateStudio(db, childName, childID, nil)
				db.Studio.On("UpdateImage", testCtx, childID, mock.AnythingOfType("[]uint8")).Return(errors.New("error updating studio image"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mocks.NewDatabase()
			if tt.setup != nil {
				tt.setup(db)
			}

			got, err := createMissingStudio(testCtx, endpoint, db.Studio, tt.studio)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_createMissingStudio_existingParentIsNotUpdated(t *testing.T) {
	childID := 1
	parentID := 2
	storedParentID := strconv.Itoa(parentID)
	parent := &models.Studio{ID: parentID, Name: "Parent", Details: "Existing details", Aliases: models.NewRelatedStrings([]string{"Parent alias"})}
	details := "Scraped details"
	aliases := "Scraped alias"
	scrapedParent := scrapedStudioWithImage("Parent alias", "data:image/png;base64,AAAA")
	scrapedParent.StoredID = &storedParentID
	scrapedParent.Details = &details
	scrapedParent.Aliases = &aliases
	scrapedParent.URLs = []string{"https://example.com/scraped"}

	db := mocks.NewDatabase()
	db.Studio.On("GetStashIDs", testCtx, parentID).Return(nil, nil).Maybe()
	db.Studio.On("Find", testCtx, parentID).Return(parent, nil).Maybe()
	db.Studio.On("Query", testCtx, mock.Anything, mock.Anything).Return([]*models.Studio{}, 0, nil).Maybe()
	db.Studio.On("UpdatePartial", testCtx, mock.Anything).Return(parent, nil).Maybe()
	db.Studio.On("UpdateImage", testCtx, parentID, mock.Anything).Return(nil).Maybe()
	db.Studio.On("Create", testCtx, mock.MatchedBy(func(input *models.CreateStudioInput) bool {
		return input.Name == "Child" && input.ParentID != nil && *input.ParentID == parentID
	})).Run(func(args mock.Arguments) {
		args.Get(1).(*models.CreateStudioInput).ID = childID
	}).Return(nil).Once()

	id, err := createMissingStudio(testCtx, "endpoint", db.Studio, &models.ScrapedStudio{
		Name: "Child", Parent: scrapedParent,
	})
	assert.NoError(t, err)
	assert.Equal(t, &childID, id)
	// No partial update preserves all existing metadata; no image update
	// preserves the stored image, including when the match used an alias.
	db.Studio.AssertNotCalled(t, "UpdatePartial", mock.Anything, mock.Anything)
	db.Studio.AssertNotCalled(t, "UpdateImage", mock.Anything, mock.Anything, mock.Anything)
	db.Studio.AssertExpectations(t)
}
