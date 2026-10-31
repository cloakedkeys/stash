package database

import (
	"github.com/stashapp/stash/pkg/models"
)

type SceneMarkerStore interface {
	models.SceneMarkerReaderWriter
}
