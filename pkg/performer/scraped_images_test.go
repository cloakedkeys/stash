package performer

import (
	"context"
	"errors"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// tiny SVGs, as data URLs and as the bytes they decode to
const (
	svgA = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0naHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmcnLz4="
	svgB = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0naHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmcnIGlkPSdiJy8+"
	svgC = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0naHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmcnIGlkPSdjJy8+"
	bad  = "data:image/png;base64,%%%%"
)

var (
	bytesB = []byte("<svg xmlns='http://www.w3.org/2000/svg' id='b'/>")
	bytesC = []byte("<svg xmlns='http://www.w3.org/2000/svg' id='c'/>")
)

func TestScrapedOtherImages(t *testing.T) {
	ctx := context.Background()
	name := "Someone"

	t.Run("loads every image after the main one, skipping broken ones", func(t *testing.T) {
		p := &models.ScrapedPerformer{Name: &name, Images: []string{svgA, svgB, bad, svgC}}
		assert.Equal(t, [][]byte{bytesB, bytesC}, ScrapedOtherImages(ctx, p, nil))
	})

	t.Run("nothing when the image is excluded", func(t *testing.T) {
		p := &models.ScrapedPerformer{Name: &name, Images: []string{svgA, svgB}}
		assert.Nil(t, ScrapedOtherImages(ctx, p, map[string]bool{"image": true}))
	})

	t.Run("nothing with one image or none", func(t *testing.T) {
		assert.Nil(t, ScrapedOtherImages(ctx, &models.ScrapedPerformer{Images: []string{svgA}}, nil))
		assert.Nil(t, ScrapedOtherImages(ctx, &models.ScrapedPerformer{}, nil))
		// the old single image field has no other images
		single := svgB
		assert.Nil(t, ScrapedOtherImages(ctx, &models.ScrapedPerformer{Image: &single}, nil))
	})
}

func TestAddAlternateImages(t *testing.T) {
	ctx := context.Background()

	t.Run("adds each image in order", func(t *testing.T) {
		db := mocks.NewDatabase()
		var added [][]byte
		db.Performer.On("AddAlternateImage", ctx, 3, mock.Anything).Run(func(args mock.Arguments) {
			added = append(added, args.Get(2).([]byte))
		}).Return(true, nil)
		require.NoError(t, AddAlternateImages(ctx, db.Performer, 3, [][]byte{bytesB, bytesC}))
		assert.Equal(t, [][]byte{bytesB, bytesC}, added)
	})

	t.Run("an image the performer already has is fine", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.Performer.On("AddAlternateImage", ctx, 3, bytesB).Return(false, nil).Once()
		require.NoError(t, AddAlternateImages(ctx, db.Performer, 3, [][]byte{bytesB}))
		db.AssertExpectations(t)
	})

	t.Run("stops on an error", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.Performer.On("AddAlternateImage", ctx, 3, bytesB).Return(false, errors.New("disk full")).Once()
		err := AddAlternateImages(ctx, db.Performer, 3, [][]byte{bytesB, bytesC})
		assert.ErrorContains(t, err, "disk full")
		db.AssertExpectations(t)
	})
}
