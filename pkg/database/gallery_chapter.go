package database

import (
	"github.com/stashapp/stash/pkg/models"
)

type GalleryChapterStore interface {
	models.GalleryChapterReaderWriter
}
