package database

import "github.com/stashapp/stash/pkg/models"

type StoryStore interface{ models.StoryReaderWriter }
