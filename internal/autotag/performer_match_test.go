package autotag

import (
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
)

func TestPerformerCandidatesSkipOneLetterNames(t *testing.T) {
	// Owner decision for TAG-1: skip one-letter names for every file type,
	// even when the SQL query returns a matching candidate.
	for _, tc := range []struct {
		name string
		run  func(*tagger, *mocks.Database, func()) error
	}{
		{"scenes", func(tt *tagger, db *mocks.Database, matched func()) error {
			items := []*models.Scene{{ID: 1, Path: "/Q.mp4"}}
			db.Scene.On("Query", mock.Anything, mock.Anything).Return(mocks.SceneQueryResult(items, 1), nil).Once()
			return tt.tagScenes(testCtx, nil, db.Scene, func(*models.Scene) (bool, error) { matched(); return false, nil })
		}},
		{"audio", func(tt *tagger, db *mocks.Database, matched func()) error {
			items := []*models.Audio{{ID: 1, Path: "/Q.mp3"}}
			db.Audio.On("Query", mock.Anything, mock.Anything).Return(mocks.AudioQueryResult(items, 1), nil).Once()
			return tt.tagAudios(testCtx, nil, db.Audio, func(*models.Audio) (bool, error) { matched(); return false, nil })
		}},
		{"images", func(tt *tagger, db *mocks.Database, matched func()) error {
			items := []*models.Image{{ID: 1, Path: "/Q.jpg"}}
			db.Image.On("Query", mock.Anything, mock.Anything).Return(mocks.ImageQueryResult(items, 1), nil).Once()
			return tt.tagImages(testCtx, nil, db.Image, func(*models.Image) (bool, error) { matched(); return false, nil })
		}},
		{"galleries", func(tt *tagger, db *mocks.Database, matched func()) error {
			items := []*models.Gallery{{ID: 1, Path: "/Q.zip"}}
			db.Gallery.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(items, 1, nil).Once()
			return tt.tagGalleries(testCtx, nil, db.Gallery, func(*models.Gallery) (bool, error) { matched(); return false, nil })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := mocks.NewDatabase()
			tt := getPerformerTaggers(&models.Performer{ID: 1, Name: "Q"}, nil)[0]
			if err := tc.run(&tt, db, func() { t.Error("one-letter performer matched") }); err != nil {
				t.Fatal(err)
			}
			db.AssertExpectations(t)
		})
	}
}
