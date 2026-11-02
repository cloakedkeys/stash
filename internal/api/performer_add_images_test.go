package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/utils"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Saving a performer from a scrape keeps the images that weren't picked as the main
// one: add_images adds them to the performer's other images, in order. One that
// can't be loaded is skipped rather than failing the save.
func TestPerformerSaveAddsImages(t *testing.T) {
	fromURL := []byte(`<svg><title>from a link</title></svg>`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write(fromURL)
		require.NoError(t, err)
	}))
	defer server.Close()
	inline := []byte(`<svg><title>inline</title></svg>`)
	add := []string{
		"data:image/svg+xml;base64," + utils.GetBase64StringFromData(inline),
		"data:text/html;base64,PGh0bWw+",
		server.URL,
	}
	addJSON, err := json.Marshal(add)
	require.NoError(t, err)

	expectAdded := func(db *mocks.Database, id int) {
		// the checks a save makes on the performer's name and aliases
		db.Performer.On("GetAliases", mock.Anything, id).Return([]string{}, nil).Maybe()
		db.Performer.On("QueryCount", mock.Anything, mock.Anything, mock.Anything).Return(0, nil).Maybe()
		first := db.Performer.On("AddAlternateImage", mock.Anything, id, inline).Return(true, nil).Once()
		db.Performer.On("AddAlternateImage", mock.Anything, id, fromURL).Return(true, nil).Once().NotBefore(first)
	}

	t.Run("update", func(t *testing.T) {
		db := mocks.NewDatabase()
		r := newResolver(db)
		db.Performer.On("UpdatePartial", mock.Anything, 1, mock.Anything).Return(&models.Performer{ID: 1}, nil).Once()
		db.Performer.On("Find", mock.Anything, 1).Return(&models.Performer{ID: 1}, nil)
		expectAdded(db, 1)

		result := performerImageQuery(t, newTestGraphQLServer(r), `mutation { performerUpdate(input:{id:"1", add_images:`+string(addJSON)+`}) { id } }`, models.RoleEnumModify)
		require.NotContains(t, result, "errors")
		db.AssertExpectations(t)
	})

	t.Run("create", func(t *testing.T) {
		db := mocks.NewDatabase()
		r := newResolver(db)
		db.Performer.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Get(1).(*models.CreatePerformerInput).Performer.ID = 2
		}).Return(nil).Once()
		db.Performer.On("Find", mock.Anything, 2).Return(&models.Performer{ID: 2}, nil)
		expectAdded(db, 2)

		result := performerImageQuery(t, newTestGraphQLServer(r), `mutation { performerCreate(input:{name:"Scraped", add_images:`+string(addJSON)+`}) { id } }`, models.RoleEnumModify)
		require.NotContains(t, result, "errors")
		db.AssertExpectations(t)
	})
}
