package api

import (
	"errors"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// The database refuses user data for a missing object with its own error, which
// names its command. These mocks give that error if the resolver gets that far.
var errForeignKey = errors.New("upserting performers_user_data: executing `INSERT ...`: FOREIGN KEY constraint failed")

func userDataMissingMocks(r *Resolver, db *mocks.Database, id int) {
	r.repository.VoiceActor = &apiVoiceActorStore{}
	db.Scene.On("Find", mock.Anything, id).Return(nil, nil)
	db.Scene.On("UpdatePartial", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Audio.On("Find", mock.Anything, id).Return(nil, nil)
	db.Audio.On("UpdatePartial", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Image.On("Find", mock.Anything, id).Return(nil, nil)
	db.Image.On("UpdatePartial", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Gallery.On("Find", mock.Anything, id).Return(nil, nil)
	db.Gallery.On("UpdatePartial", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Group.On("Find", mock.Anything, id).Return(nil, nil)
	db.Group.On("UpdatePartial", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Performer.On("Find", mock.Anything, id).Return(nil, nil)
	db.Performer.On("UpdatePartial", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Studio.On("Find", mock.Anything, id).Return(nil, nil)
	db.Studio.On("UpdatePartial", mock.Anything, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Tag.On("Find", mock.Anything, id).Return(nil, nil)
	db.Tag.On("UpdatePartial", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Story.On("Find", mock.Anything, id).Return(nil, nil)
}

var userDataMissingMutations = map[string]string{
	`sceneUserDataUpdate(input:{id:"999",rating100:80}) { id }`:       "scene with id 999 not found",
	`audioUserDataUpdate(input:{id:"999",rating100:80}) { id }`:       "audio with id 999 not found",
	`imageUserDataUpdate(input:{id:"999",rating100:80}) { id }`:       "image with id 999 not found",
	`galleryUserDataUpdate(input:{id:"999",rating100:80}) { id }`:     "gallery with id 999 not found",
	`groupUserDataUpdate(input:{id:"999",rating100:80}) { id }`:       "group with id 999 not found",
	`performerUserDataUpdate(input:{id:"999",rating100:80}) { id }`:   "performer with id 999 not found",
	`performerUserDataUpdate(input:{id:"999",favorite:true}) { id }`:  "performer with id 999 not found",
	`studioUserDataUpdate(input:{id:"999",rating100:80}) { id }`:      "studio with id 999 not found",
	`tagUserDataUpdate(input:{id:"999",favorite:true}) { id }`:        "tag with id 999 not found",
	`storyUserDataUpdate(input:{id:"999",rating100:80}) { id }`:       "story with id 999 not found",
	`voiceActorUserDataUpdate(input:{id:"999",favorite:true}) { id }`: "voice actor 999 not found",
}

func TestUserDataUpdateMissingObject(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	gql := newTestGraphQLServer(r)
	gql.AroundFields(r.LibraryLimitMiddleware)
	userDataMissingMocks(r, db, 999)

	reader := models.User{ID: 2, Username: "reader", Roles: models.Roles{models.RoleEnumRead}}
	for mutation, want := range userDataMissingMutations {
		body := storyRequest(t, gql, reader, "mutation { "+mutation+" }")
		require.Contains(t, body, `"message":"`+want+`"`, mutation)
		require.NotContains(t, body, "FOREIGN KEY", mutation)
	}

	// nothing is written for an object that isn't there
	db.Scene.AssertNotCalled(t, "UpdatePartial", mock.Anything, mock.Anything, mock.Anything)
	db.Performer.AssertNotCalled(t, "UpdatePartial", mock.Anything, mock.Anything, mock.Anything)
	db.Studio.AssertNotCalled(t, "UpdatePartial", mock.Anything, mock.Anything)
}

// An account with a library limit gets the same answer for a hidden object as for a
// missing one, so it can't find out what exists. The limit is checked before the
// resolver, so it never sees the resolver's longer message.
func TestUserDataUpdateMissingObjectWithLibraryLimit(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	gql := newTestGraphQLServer(r)
	gql.AroundFields(r.LibraryLimitMiddleware)
	userDataMissingMocks(r, db, 999)

	limit := &models.LibraryLimit{TagIDs: []int{7}}
	reader := models.User{ID: 3, Username: "reader", Roles: models.Roles{models.RoleEnumRead}, LibraryLimit: limit}
	for mutation := range userDataMissingMutations {
		body := storyRequest(t, gql, reader, "mutation { "+mutation+" }")
		require.Contains(t, body, `"message":"not found"`, mutation)
		require.NotContains(t, body, "with id", mutation)
		require.NotContains(t, body, "999 not found", mutation)
	}
}
