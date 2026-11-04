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

	// O and play history and resume points
	db.Scene.On("AddO", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Scene.On("DeleteO", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Scene.On("ResetO", mock.Anything, id).Maybe().Return(0, errForeignKey)
	db.Scene.On("AddViews", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Scene.On("DeleteViews", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Scene.On("DeleteAllViews", mock.Anything, id).Maybe().Return(0, errForeignKey)
	db.Scene.On("SaveActivity", mock.Anything, id, mock.Anything, mock.Anything).Maybe().Return(false, errForeignKey)
	db.Scene.On("ResetActivity", mock.Anything, id, mock.Anything, mock.Anything).Maybe().Return(false, errForeignKey)
	db.Audio.On("AddO", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Audio.On("DeleteO", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Audio.On("ResetO", mock.Anything, id).Maybe().Return(0, errForeignKey)
	db.Audio.On("AddViews", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Audio.On("DeleteViews", mock.Anything, id, mock.Anything).Maybe().Return(nil, errForeignKey)
	db.Audio.On("DeleteAllViews", mock.Anything, id).Maybe().Return(0, errForeignKey)
	db.Audio.On("SaveActivity", mock.Anything, id, mock.Anything, mock.Anything).Maybe().Return(false, errForeignKey)
	db.Audio.On("ResetActivity", mock.Anything, id, mock.Anything, mock.Anything).Maybe().Return(false, errForeignKey)
	db.Image.On("IncrementOCounter", mock.Anything, id).Maybe().Return(0, errForeignKey)
	db.Image.On("DecrementOCounter", mock.Anything, id).Maybe().Return(0, errForeignKey)
	db.Image.On("ResetOCounter", mock.Anything, id).Maybe().Return(0, errForeignKey)
	db.Gallery.On("IncrementOCounter", mock.Anything, id).Maybe().Return(0, errForeignKey)
	db.Gallery.On("DecrementOCounter", mock.Anything, id).Maybe().Return(0, errForeignKey)
	db.Gallery.On("ResetOCounter", mock.Anything, id).Maybe().Return(0, errForeignKey)
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
	`voiceActorUserDataUpdate(input:{id:"999",favorite:true}) { id }`: "voice actor with id 999 not found",

	// O and play history and resume points
	`sceneIncrementO(id:"999")`:                                          "scene with id 999 not found",
	`sceneDecrementO(id:"999")`:                                          "scene with id 999 not found",
	`sceneAddO(id:"999") { count }`:                                      "scene with id 999 not found",
	`sceneDeleteO(id:"999") { count }`:                                   "scene with id 999 not found",
	`sceneResetO(id:"999")`:                                              "scene with id 999 not found",
	`sceneSaveActivity(id:"999",resume_time:5,playDuration:5)`:           "scene with id 999 not found",
	`sceneResetActivity(id:"999",reset_resume:true,reset_duration:true)`: "scene with id 999 not found",
	`sceneIncrementPlayCount(id:"999")`:                                  "scene with id 999 not found",
	`sceneAddPlay(id:"999") { count }`:                                   "scene with id 999 not found",
	`sceneDeletePlay(id:"999") { count }`:                                "scene with id 999 not found",
	`sceneResetPlayCount(id:"999")`:                                      "scene with id 999 not found",
	`audioAddO(id:"999") { count }`:                                      "audio with id 999 not found",
	`audioDeleteO(id:"999") { count }`:                                   "audio with id 999 not found",
	`audioResetO(id:"999")`:                                              "audio with id 999 not found",
	`audioSaveActivity(id:"999",resume_time:5,playDuration:5)`:           "audio with id 999 not found",
	`audioResetActivity(id:"999",reset_resume:true,reset_duration:true)`: "audio with id 999 not found",
	`audioAddPlay(id:"999") { count }`:                                   "audio with id 999 not found",
	`audioDeletePlay(id:"999") { count }`:                                "audio with id 999 not found",
	`audioResetPlayCount(id:"999")`:                                      "audio with id 999 not found",
	`imageIncrementO(id:"999")`:                                          "image with id 999 not found",
	`imageDecrementO(id:"999")`:                                          "image with id 999 not found",
	`imageResetO(id:"999")`:                                              "image with id 999 not found",
	`galleryIncrementO(id:"999")`:                                        "gallery with id 999 not found",
	`galleryDecrementO(id:"999")`:                                        "gallery with id 999 not found",
	`galleryResetO(id:"999")`:                                            "gallery with id 999 not found",
}

// The edit calls need an account that can change the library, so a limited account
// can't use them at all.
var missingObjectUpdateMutations = map[string]string{
	`bulkSceneUpdate(input:{ids:["999"],rating100:80}) { id }`:   "scene with id 999 not found",
	`bulkAudioUpdate(input:{ids:["999"],rating100:80}) { id }`:   "audio with id 999 not found",
	`bulkGalleryUpdate(input:{ids:["999"],rating100:80}) { id }`: "gallery with id 999 not found",
	`groupUpdate(input:{id:"999",rating100:80}) { id }`:          "group with id 999 not found",
	`bulkGroupUpdate(input:{ids:["999"],rating100:80}) { id }`:   "group with id 999 not found",
	`movieUpdate(input:{id:"999",rating100:80}) { id }`:          "movie with id 999 not found",
	`bulkMovieUpdate(input:{ids:["999"],rating100:80}) { id }`:   "movie with id 999 not found",
	`voiceActorUpdate(input:{id:"999",favorite:true}) { id }`:    "voice actor with id 999 not found",
}

func TestUserDataUpdateMissingObject(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	gql := newTestGraphQLServer(r)
	gql.AroundFields(r.LibraryLimitMiddleware)
	userDataMissingMocks(r, db, 999)

	reader := models.User{ID: 2, Username: "reader", Roles: models.Roles{models.RoleEnumRead}}
	for mutation, want := range userDataMissingMutations {
		t.Run(mutation, func(t *testing.T) {
			body := storyRequest(t, gql, reader, "mutation { "+mutation+" }")
			require.Contains(t, body, `"message":"`+want+`"`)
			require.NotContains(t, body, "FOREIGN KEY")
		})
	}

	// nothing is written for an object that isn't there
	db.Scene.AssertNotCalled(t, "UpdatePartial", mock.Anything, mock.Anything, mock.Anything)
	db.Performer.AssertNotCalled(t, "UpdatePartial", mock.Anything, mock.Anything, mock.Anything)
	db.Studio.AssertNotCalled(t, "UpdatePartial", mock.Anything, mock.Anything)
	db.Scene.AssertNotCalled(t, "AddO", mock.Anything, mock.Anything, mock.Anything)
	db.Scene.AssertNotCalled(t, "SaveActivity", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	db.Audio.AssertNotCalled(t, "AddViews", mock.Anything, mock.Anything, mock.Anything)
	db.Image.AssertNotCalled(t, "IncrementOCounter", mock.Anything, mock.Anything)
	db.Gallery.AssertNotCalled(t, "IncrementOCounter", mock.Anything, mock.Anything)
}

func TestUpdateMissingObject(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	gql := newTestGraphQLServer(r)
	gql.AroundFields(r.LibraryLimitMiddleware)
	userDataMissingMocks(r, db, 999)

	editor := models.User{ID: 2, Username: "editor", Roles: models.Roles{models.RoleEnumModify}}
	for mutation, want := range missingObjectUpdateMutations {
		t.Run(mutation, func(t *testing.T) {
			body := storyRequest(t, gql, editor, "mutation { "+mutation+" }")
			require.Contains(t, body, `"message":"`+want+`"`)
			require.NotContains(t, body, "FOREIGN KEY")
		})
	}

	// nothing is written for an object that isn't there
	db.Scene.AssertNotCalled(t, "UpdatePartial", mock.Anything, mock.Anything, mock.Anything)
	db.Audio.AssertNotCalled(t, "UpdatePartial", mock.Anything, mock.Anything, mock.Anything)
	db.Gallery.AssertNotCalled(t, "UpdatePartial", mock.Anything, mock.Anything, mock.Anything)
	db.Group.AssertNotCalled(t, "UpdatePartial", mock.Anything, mock.Anything, mock.Anything)
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
