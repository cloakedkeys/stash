package database

import (
	"github.com/stashapp/stash/pkg/models"
)

type FileStore interface {
	models.FileReaderWriter
}
