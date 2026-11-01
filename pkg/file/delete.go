package file

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
)

const (
	deleteFileSuffix   = ".delete"
	rewrittenZipSuffix = ".rewritten"
)

// RenamerRemover provides access to the Rename and Remove functions.
type RenamerRemover interface {
	Renamer
	Remove(name string) error
	RemoveAll(path string) error
	Statter
}

type renamerRemoverImpl struct {
	RenameFn    func(oldpath, newpath string) error
	RemoveFn    func(name string) error
	RemoveAllFn func(path string) error
	StatFn      func(path string) (fs.FileInfo, error)
}

func (r renamerRemoverImpl) Rename(oldpath, newpath string) error {
	return r.RenameFn(oldpath, newpath)
}

func (r renamerRemoverImpl) Remove(name string) error {
	return r.RemoveFn(name)
}

func (r renamerRemoverImpl) RemoveAll(path string) error {
	return r.RemoveAllFn(path)
}

func (r renamerRemoverImpl) Stat(path string) (fs.FileInfo, error) {
	return r.StatFn(path)
}

func newRenamerRemoverImpl() renamerRemoverImpl {
	return renamerRemoverImpl{
		// use fsutil.SafeMove to support cross-device moves
		RenameFn:    fsutil.SafeMove,
		RemoveFn:    os.Remove,
		RemoveAllFn: os.RemoveAll,
		StatFn:      os.Stat,
	}
}

// Deleter is used to safely delete files and directories from the filesystem.
// During a transaction, files and directories are marked for deletion using
// the Files and Dirs methods. If TrashPath is set, files are moved to trash
// immediately. Otherwise, they are renamed with a .delete suffix. If the
// transaction is rolled back, then the files/directories can be restored to
// their original state with the Rollback method. If the transaction is
// committed, the marked files are then deleted from the filesystem using the
// Commit method.
type Deleter struct {
	RenamerRemover RenamerRemover
	files          []string
	dirs           []string
	TrashPath      string                 // if set, files will be moved to this directory instead of being permanently deleted
	trashedPaths   map[string]string      // map of original path -> trash path (only used when TrashPath is set)
	zips           map[string]*zipRewrite // zipPath -> entries to remove from that zip
}

type zipRewrite struct {
	id      models.FileID
	pending []string // entries not yet removed
	// rewritten in place in this transaction; the original was set aside with
	// renameForDelete and is in files
	replaced bool
}

// ZipFileStore finds and updates the zip's own file record.
type ZipFileStore interface {
	Find(ctx context.Context, id ...models.FileID) ([]models.File, error)
	Update(ctx context.Context, f models.File) error
}

func NewDeleter() *Deleter {
	return &Deleter{
		RenamerRemover: newRenamerRemoverImpl(),
		TrashPath:      "",
		trashedPaths:   make(map[string]string),
	}
}

func NewDeleterWithTrash(trashPath string) *Deleter {
	return &Deleter{
		RenamerRemover: newRenamerRemoverImpl(),
		TrashPath:      trashPath,
		trashedPaths:   make(map[string]string),
	}
}

// RegisterHooks registers post-commit and post-rollback hooks.
func (d *Deleter) RegisterHooks(ctx context.Context) {
	txn.AddPostCommitHook(ctx, func(ctx context.Context) {
		d.Commit()
	})

	txn.AddPostRollbackHook(ctx, func(ctx context.Context) {
		d.Rollback()
	})
}

// Files designates files to be deleted. Each file marked will be renamed to add
// a `.delete` suffix. An error is returned if a file could not be renamed.
// Note that if an error is returned, then some files may be left renamed.
// Abort should be called to restore marked files if this function returns an
// error.
func (d *Deleter) Files(paths []string) error {
	return d.filesInternal(paths, false)
}

// FilesWithoutTrash designates files to be deleted, bypassing the trash directory.
// Files will be permanently deleted even if TrashPath is configured.
// This is useful for deleting generated files that can be easily recreated.
func (d *Deleter) FilesWithoutTrash(paths []string) error {
	return d.filesInternal(paths, true)
}

func (d *Deleter) filesInternal(paths []string, bypassTrash bool) error {
	for _, p := range paths {
		// fail silently if the file does not exist
		if _, err := d.RenamerRemover.Stat(p); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				logger.Warnf("File %q does not exist and therefore cannot be deleted. Ignoring.", p)
				continue
			}

			return fmt.Errorf("check file %q exists: %w", p, err)
		}

		if err := d.renameForDelete(p, bypassTrash); err != nil {
			return fmt.Errorf("marking file %q for deletion: %w", p, err)
		}
		d.files = append(d.files, p)
	}

	return nil
}

// Dirs designates directories to be deleted. Each directory marked will be renamed to add
// a `.delete` suffix. An error is returned if a directory could not be renamed.
// Note that if an error is returned, then some directories may be left renamed.
// Abort should be called to restore marked files/directories if this function returns an
// error.
func (d *Deleter) Dirs(paths []string) error {
	return d.dirsInternal(paths, false)
}

// DirsWithoutTrash designates directories to be deleted, bypassing the trash directory.
// Directories will be permanently deleted even if TrashPath is configured.
// This is useful for deleting generated directories that can be easily recreated.
func (d *Deleter) DirsWithoutTrash(paths []string) error {
	return d.dirsInternal(paths, true)
}

func (d *Deleter) dirsInternal(paths []string, bypassTrash bool) error {
	for _, p := range paths {
		// fail silently if the file does not exist
		if _, err := d.RenamerRemover.Stat(p); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				logger.Warnf("Directory %q does not exist and therefore cannot be deleted. Ignoring.", p)
				continue
			}

			return fmt.Errorf("check directory %q exists: %w", p, err)
		}

		if err := d.renameForDelete(p, bypassTrash); err != nil {
			return fmt.Errorf("marking directory %q for deletion: %w", p, err)
		}
		d.dirs = append(d.dirs, p)
	}

	return nil
}

// ZipEntry marks a single entry within a zip for removal. PrepareZipRewrites must be
// called before the transaction commits.
// entryRelPath must be the relative path of the entry within the zip using forward slashes.
func (d *Deleter) ZipEntry(zipID models.FileID, zipPath, entryRelPath string) {
	if d.zips == nil {
		d.zips = make(map[string]*zipRewrite)
	}
	z := d.zips[zipPath]
	if z == nil {
		z = &zipRewrite{id: zipID}
		d.zips[zipPath] = z
	}
	z.pending = append(z.pending, entryRelPath)
}

// PrepareZipRewrites removes the marked entries from their zips, once per zip, and
// stores each zip's new size, or calls destroyEmptyZip to remove its database records
// if no files remain in the archive. Call it inside the transaction, after all entries are
// marked. The original zip is set aside like any other deleted file: moved to the
// trash, or renamed with a .delete suffix. The rewritten zip takes its place right
// away, so the library and the archive match as soon as the transaction commits. If
// anything fails, the error rolls the whole deletion back, and Rollback puts the
// original zip back. The stored modification time is left as it was, so the next scan
// sees the zip changed and refreshes its fingerprints.
func (d *Deleter) PrepareZipRewrites(ctx context.Context, files ZipFileStore, destroyEmptyZip func(context.Context, models.File) error) error {
	for zipPath, z := range d.zips {
		if len(z.pending) == 0 {
			continue
		}
		// a zip deleted whole in this transaction has nothing left to rewrite
		if !z.replaced && slices.Contains(d.files, zipPath) {
			z.pending = nil
			continue
		}
		if err := d.rewriteZip(ctx, files, zipPath, z, destroyEmptyZip); err != nil {
			return err
		}
		z.pending = nil
	}

	return nil
}

func (d *Deleter) rewriteZip(ctx context.Context, files ZipFileStore, zipPath string, z *zipRewrite, destroyEmptyZip func(context.Context, models.File) error) error {
	entries := z.pending
	found, err := files.Find(ctx, z.id)
	if err != nil {
		return fmt.Errorf("finding zip %q: %w", zipPath, err)
	}
	if len(found) == 0 {
		return fmt.Errorf("zip %q is not in the database", zipPath)
	}
	zf := found[0]

	tmpPath, err := RemoveEntriesFromZip(zipPath, entries)
	if errors.Is(err, errNothingToRemove) {
		return nil
	}
	if err != nil {
		return err
	}

	if tmpPath == "" && destroyEmptyZip == nil {
		return fmt.Errorf("no empty zip destroyer for %q", zipPath)
	}

	if z.replaced {
		// already rewritten in this transaction: the original is set aside, so the
		// earlier rewrite can simply go
		err = d.RenamerRemover.Remove(zipPath)
	} else {
		err = d.renameForDelete(zipPath, false)
		if err == nil {
			d.files = append(d.files, zipPath)
			z.replaced = true
		}
	}
	if err != nil {
		if tmpPath != "" {
			_ = d.RenamerRemover.Remove(tmpPath)
		}
		return fmt.Errorf("setting aside %q: %w", zipPath, err)
	}

	if tmpPath == "" {
		// The original is already set aside for rollback and trash handling.
		return destroyEmptyZip(ctx, zf)
	}

	if err := d.RenamerRemover.Rename(tmpPath, zipPath); err != nil {
		_ = d.RenamerRemover.Remove(tmpPath)
		return fmt.Errorf("replacing %q: %w", zipPath, err)
	}

	info, err := d.RenamerRemover.Stat(zipPath)
	if err != nil {
		return fmt.Errorf("reading rewritten zip %q: %w", zipPath, err)
	}

	// the zip is opened with its stored size, so it must match the new file
	zf.Base().Size = info.Size()
	if err := files.Update(ctx, zf); err != nil {
		return fmt.Errorf("updating zip %q: %w", zipPath, err)
	}

	logger.Infof("Removed %d entries from %q", len(entries), zipPath)
	return nil
}

// Rollback tries to rename all marked files and directories back to their
// original names and clears the marked list. Any errors encountered are
// logged. All files will be attempted regardless of any errors occurred.
func (d *Deleter) Rollback() {
	// a rewritten zip is moved out of the way so its original can come back to the
	// same path, and only removed once the original is back
	rewritten := make(map[string]string)
	for zipPath, z := range d.zips {
		if !z.replaced {
			continue
		}
		aside := zipPath + rewrittenZipSuffix
		if err := d.RenamerRemover.Rename(zipPath, aside); err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				logger.Warnf("Error moving rewritten zip %q aside: %v", zipPath, err)
			}
			continue
		}
		rewritten[zipPath] = aside
	}

	for _, f := range append(d.files, d.dirs...) {
		if err := d.renameForRestore(f); err != nil {
			logger.Warnf("Error restoring %q: %v", f, err)
			if aside, ok := rewritten[f]; ok {
				// better the rewritten zip than none at all
				if err := d.RenamerRemover.Rename(aside, f); err != nil {
					logger.Errorf("Error putting back %q: %v. The original zip is at %q.", f, err, d.setAsidePath(f))
				}
				delete(rewritten, f)
			}
			continue
		}
		if aside, ok := rewritten[f]; ok {
			if err := d.RenamerRemover.Remove(aside); err != nil {
				logger.Warnf("Error removing rewritten zip %q: %v", aside, err)
			}
		}
	}

	d.files = nil
	d.dirs = nil
	d.trashedPaths = make(map[string]string)
	d.clearZips()
}

func (d *Deleter) clearZips() {
	d.zips = nil
}

// setAsidePath is where renameForDelete put path.
func (d *Deleter) setAsidePath(path string) string {
	if p, ok := d.trashedPaths[path]; ok {
		return p
	}
	return path + deleteFileSuffix
}

// Commit deletes all files marked for deletion and clears the marked list.
// When using trash, files have already been moved during renameForDelete, so
// this just clears the tracking. Otherwise, permanently delete the .delete files.
// Any errors encountered are logged. All files will be attempted, regardless
// of the errors encountered.
func (d *Deleter) Commit() {
	if d.TrashPath != "" {
		// Files were already moved to trash during renameForDelete, just clear tracking
		logger.Debugf("Commit: %d files and %d directories already in trash, clearing tracking", len(d.files), len(d.dirs))
	} else {
		// Permanently delete files and directories marked with .delete suffix
		for _, f := range d.files {
			if err := d.RenamerRemover.Remove(f + deleteFileSuffix); err != nil {
				logger.Warnf("Error deleting file %q: %v", f+deleteFileSuffix, err)
			}
		}

		for _, f := range d.dirs {
			if err := d.RenamerRemover.RemoveAll(f + deleteFileSuffix); err != nil {
				logger.Warnf("Error deleting directory %q: %v", f+deleteFileSuffix, err)
			}
		}
	}

	// rewritten zips are already in place, and their originals were handled above
	for zipPath, z := range d.zips {
		if len(z.pending) > 0 {
			logger.Warnf("Images in %q were removed from the library but not from the zip, so a scan will add them back", zipPath)
		}
	}

	d.files = nil
	d.dirs = nil
	d.trashedPaths = make(map[string]string)
	d.clearZips()
}

func (d *Deleter) renameForDelete(path string, bypassTrash bool) error {
	if d.TrashPath != "" && !bypassTrash {
		// Move file to trash immediately
		trashDest, err := fsutil.MoveToTrash(path, d.TrashPath)
		if err != nil {
			return err
		}
		d.trashedPaths[path] = trashDest
		logger.Infof("Moved %q to trash at %s", path, trashDest)
		return nil
	}

	// Standard behavior: rename with .delete suffix (or when bypassing trash)
	return d.RenamerRemover.Rename(path, path+deleteFileSuffix)
}

func (d *Deleter) renameForRestore(path string) error {
	if d.TrashPath != "" {
		// Restore file from trash
		trashPath, ok := d.trashedPaths[path]
		if !ok {
			return fmt.Errorf("no trash path found for %q", path)
		}
		return d.RenamerRemover.Rename(trashPath, path)
	}

	// Standard behavior: restore from .delete suffix
	return d.RenamerRemover.Rename(path+deleteFileSuffix, path)
}

func Destroy(ctx context.Context, destroyer models.FileDestroyer, f models.File, fileDeleter *Deleter, deleteFile bool) error {
	if err := destroyer.Destroy(ctx, f.Base().ID); err != nil {
		return err
	}

	// don't delete files in zip files
	if deleteFile && f.Base().ZipFileID == nil {
		if err := fileDeleter.Files([]string{f.Base().Path}); err != nil {
			return err
		}
	}

	return nil
}

type ZipDestroyer struct {
	FileDestroyer   models.FileFinderDestroyer
	FolderDestroyer models.FolderFinderDestroyer
}

func (d *ZipDestroyer) DestroyZip(ctx context.Context, f models.File, fileDeleter *Deleter, deleteFile bool) error {
	// destroy contained files
	files, err := d.FileDestroyer.FindByZipFileID(ctx, f.Base().ID)
	if err != nil {
		return err
	}

	for _, ff := range files {
		if err := d.FileDestroyer.Destroy(ctx, ff.Base().ID); err != nil {
			return err
		}
	}

	// destroy contained folders
	folders, err := d.FolderDestroyer.FindByZipFileID(ctx, f.Base().ID)
	if err != nil {
		return err
	}

	for _, ff := range folders {
		if err := d.FolderDestroyer.Destroy(ctx, ff.ID); err != nil {
			return err
		}
	}

	if err := d.FileDestroyer.Destroy(ctx, f.Base().ID); err != nil {
		return err
	}

	if deleteFile {
		if err := fileDeleter.Files([]string{f.Base().Path}); err != nil {
			return err
		}
	}

	return nil
}
