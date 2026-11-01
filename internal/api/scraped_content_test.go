package api

import (
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestMarshalScrapedImageNoResult(t *testing.T) {
	image, err := marshalScrapedImage(nil)
	require.NoError(t, err)
	require.Nil(t, image)

	title := "Image"
	want := &models.ScrapedImage{Title: &title}
	image, err = marshalScrapedImage(want)
	require.NoError(t, err)
	require.Equal(t, want, image)
}
