package file

import (
	"context"
	"fmt"
	"time"

	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

// MissingPurger purges files and folders marked as missing and their associated objects from the database.
type MissingPurger struct {
	Repository Repository

	PurgeHandlers []PurgeHandler
	TrashPath     string
}

type purgeMissingJob struct {
	*MissingPurger

	progress *job.Progress
	options  PurgeMissingOptions
}

// PurgeMissingOptions provides options for purging missing files.
type PurgeMissingOptions struct {
	Paths []string

	// DryRun indicates if this is a dry run. A dry run will not make any changes and will only log what would have been done.
	DryRun bool

	// MissingSinceBefore is an optional filter to only purge files and folders that have been marked as missing since before the specified time.
	MissingSinceBefore *time.Time
}

// PurgeMissing starts the purge missing process.
func (s *MissingPurger) PurgeMissing(ctx context.Context, options PurgeMissingOptions, progress *job.Progress) {
	j := &purgeMissingJob{
		MissingPurger: s,
		progress:      progress,
		options:       options,
	}

	if err := j.execute(ctx); err != nil {
		logger.Errorf("error purging missing files: %v", err)
		return
	}
}

func (j *purgeMissingJob) execute(ctx context.Context) error {
	progress := j.progress

	var (
		fileCount   int
		folderCount int
	)

	r := j.Repository
	if err := r.WithReadTxn(ctx, func(ctx context.Context) error {
		var err error
		fileCount, err = r.File.CountMissingInPaths(ctx, j.options.Paths, j.options.MissingSinceBefore)
		if err != nil {
			return err
		}

		folderCount, err = r.Folder.CountMissingInPaths(ctx, j.options.Paths, j.options.MissingSinceBefore)
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	progress.AddTotal(fileCount + folderCount)
	progress.Definite()

	if err := j.purgeMissingFiles(ctx, progress); err != nil {
		return err
	}

	if err := j.purgeMissingFolders(ctx); err != nil {
		return err
	}

	return nil
}

// purgeEntry is a file or folder to be purged.
type purgeEntry struct {
	id   int
	path string
}

const purgeBatchSize = 1000

// findMissingFiles returns all files to purge. The full list is collected before anything is deleted,
// so that a file that fails to delete is not returned again, which would never finish.
func (j *purgeMissingJob) findMissingFiles(ctx context.Context) ([]purgeEntry, error) {
	var ret []purgeEntry
	r := j.Repository
	if err := r.WithReadTxn(ctx, func(ctx context.Context) error {
		for offset := 0; ; offset += purgeBatchSize {
			if job.IsCancelled(ctx) {
				return nil
			}

			files, err := r.File.FindMissingInPaths(ctx, j.options.Paths, j.options.MissingSinceBefore, purgeBatchSize, offset)
			if err != nil {
				return fmt.Errorf("error querying for files: %w", err)
			}

			for _, f := range files {
				ret = append(ret, purgeEntry{id: int(f.Base().ID), path: f.Base().Path})
			}

			if len(files) != purgeBatchSize {
				return nil
			}
		}
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

// findMissingFolders returns all folders to purge. See findMissingFiles.
func (j *purgeMissingJob) findMissingFolders(ctx context.Context) ([]purgeEntry, error) {
	var ret []purgeEntry
	r := j.Repository
	if err := r.WithReadTxn(ctx, func(ctx context.Context) error {
		for offset := 0; ; offset += purgeBatchSize {
			if job.IsCancelled(ctx) {
				return nil
			}

			folders, err := r.Folder.FindMissingInPaths(ctx, j.options.Paths, j.options.MissingSinceBefore, purgeBatchSize, offset)
			if err != nil {
				return fmt.Errorf("error querying for folders: %w", err)
			}

			for _, f := range folders {
				ret = append(ret, purgeEntry{id: int(f.ID), path: f.Path})
			}

			if len(folders) != purgeBatchSize {
				return nil
			}
		}
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

func (j *purgeMissingJob) purgeMissingFiles(ctx context.Context, progress *job.Progress) error {
	files, err := j.findMissingFiles(ctx)
	if err != nil {
		return err
	}

	for _, f := range files {
		if job.IsCancelled(ctx) {
			return nil
		}

		if j.options.DryRun {
			logger.Infof("Would delete file %q from database", f.path)
		} else {
			logger.Infof("Deleting file %q from database", f.path)
			j.deleteFile(ctx, models.FileID(f.id), f.path)
		}
		progress.Increment()
	}

	return nil
}

func (j *purgeMissingJob) purgeMissingFolders(ctx context.Context) error {
	progress := j.progress

	folders, err := j.findMissingFolders(ctx)
	if err != nil {
		return err
	}

	for _, f := range folders {
		if job.IsCancelled(ctx) {
			return nil
		}

		if j.options.DryRun {
			logger.Infof("Would delete folder %q from database", f.path)
		} else {
			logger.Infof("Deleting folder %q from database", f.path)
			j.deleteFolder(ctx, models.FolderID(f.id), f.path)
		}
		progress.Increment()
	}

	return nil
}

func (j *purgeMissingJob) deleteFile(ctx context.Context, fileID models.FileID, fn string) {
	// delete associated objects
	fileDeleter := NewDeleterWithTrash(j.TrashPath)
	r := j.Repository
	if err := r.WithTxn(ctx, func(ctx context.Context) error {
		fileDeleter.RegisterHooks(ctx)

		if err := j.fireHandlers(ctx, fileDeleter, fileID); err != nil {
			return err
		}

		return r.File.Destroy(ctx, fileID)
	}); err != nil {
		logger.Errorf("Error deleting file %q from database: %s", fn, err.Error())
		return
	}
}

func (j *purgeMissingJob) deleteFolder(ctx context.Context, folderID models.FolderID, fn string) {
	// delete associated objects
	fileDeleter := NewDeleterWithTrash(j.TrashPath)
	r := j.Repository
	if err := r.WithTxn(ctx, func(ctx context.Context) error {
		fileDeleter.RegisterHooks(ctx)

		if err := j.fireFolderHandlers(ctx, fileDeleter, folderID); err != nil {
			return err
		}

		return r.Folder.Destroy(ctx, folderID)
	}); err != nil {
		logger.Errorf("Error deleting folder %q from database: %s", fn, err.Error())
		return
	}
}

func (j *purgeMissingJob) fireHandlers(ctx context.Context, fileDeleter *Deleter, fileID models.FileID) error {
	for _, h := range j.PurgeHandlers {
		if err := h.HandleFile(ctx, fileDeleter, fileID); err != nil {
			return err
		}
	}

	return nil
}

func (j *purgeMissingJob) fireFolderHandlers(ctx context.Context, fileDeleter *Deleter, folderID models.FolderID) error {
	for _, h := range j.PurgeHandlers {
		if err := h.HandleFolder(ctx, fileDeleter, folderID); err != nil {
			return err
		}
	}

	return nil
}
