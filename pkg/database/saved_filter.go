package database

import (
	"github.com/stashapp/stash/pkg/models"
)

type SavedFilterStore interface {
	models.SavedFilterReaderWriter
}
