package story

import (
	"context"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

// FileUser is anything that can use a file, such as stories, scenes or audio.
type FileUser interface {
	CountByFileID(ctx context.Context, id models.FileID) (int, error)
}

// DestroyerRepository is what Destroy needs to remove a story and, if asked, its files.
type DestroyerRepository interface {
	models.FileLoader
	FileUser
	Destroy(ctx context.Context, id int) error
}

// Destroy removes a story from the library. With deleteFile, its files are deleted too,
// both their records and, through fileDeleter, the files on disk, so the next scan
// doesn't add the story again. A file that another story, or anything in others, also
// uses is kept. A file inside a zip only loses its record, since the zip isn't changed.
func Destroy(ctx context.Context, r DestroyerRepository, files models.FileDestroyer, others []FileUser, s *models.Story, fileDeleter *file.Deleter, deleteFile bool) error {
	if deleteFile {
		if err := s.LoadFiles(ctx, r); err != nil {
			return err
		}

		for _, f := range s.Files.List() {
			used, err := usedElsewhere(ctx, r, others, f.Base().ID)
			if err != nil {
				return err
			}
			if used {
				logger.Infof("Keeping story file %s, which something else uses", f.Base().Path)
				continue
			}

			logger.Info("Deleting story file: ", f.Base().Path)
			if err := file.Destroy(ctx, files, f, fileDeleter, true); err != nil {
				return err
			}
		}
	}

	return r.Destroy(ctx, s.ID)
}

// usedElsewhere reports whether a file is used by another story, besides the one being
// removed, or by anything in others.
func usedElsewhere(ctx context.Context, stories FileUser, others []FileUser, id models.FileID) (bool, error) {
	count, err := stories.CountByFileID(ctx, id)
	if err != nil {
		return false, err
	}
	if count > 1 {
		return true, nil
	}
	for _, o := range others {
		count, err := o.CountByFileID(ctx, id)
		if err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}
