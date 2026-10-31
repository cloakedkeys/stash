package database

import "github.com/stashapp/stash/pkg/models"

type AudioStore interface {
	models.AudioReaderWriter
	viewDateManager
}
