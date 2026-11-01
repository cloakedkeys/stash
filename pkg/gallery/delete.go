package gallery

import (
	"context"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/image"
	"github.com/stashapp/stash/pkg/models"
)

func (s *Service) Destroy(ctx context.Context, i *models.Gallery, fileDeleter *image.FileDeleter, deleteGenerated, deleteFile, destroyFileEntry bool) ([]*models.Image, error) {
	var imgsDestroyed []*models.Image

	// chapter deletion is done via delete cascade, so we don't need to do anything here

	// if this is a zip-based gallery, delete the images as well first
	zipImgsDestroyed, err := s.destroyZipFileImages(ctx, i, fileDeleter, deleteGenerated, deleteFile, destroyFileEntry)
	if err != nil {
		return nil, err
	}

	imgsDestroyed = zipImgsDestroyed

	// only delete folder based gallery images if we're deleting the folder
	if deleteFile && i.FolderID != nil {
		folderImgsDestroyed, err := s.ImageService.DestroyFolderImages(ctx, *i.FolderID, fileDeleter, deleteGenerated, deleteFile)
		if err != nil {
			return nil, err
		}

		imgsDestroyed = append(imgsDestroyed, folderImgsDestroyed...)
	}

	// we only want to delete a folder-based gallery if it is empty.
	// this has to be done post-transaction

	if err := s.Repository.Destroy(ctx, i.ID); err != nil {
		return nil, err
	}

	return imgsDestroyed, nil
}

func DestroyChapter(ctx context.Context, galleryChapter *models.GalleryChapter, qb models.GalleryChapterDestroyer) error {
	return qb.Destroy(ctx, galleryChapter.ID)
}

func (s *Service) destroyZipFileImages(ctx context.Context, i *models.Gallery, fileDeleter *image.FileDeleter, deleteGenerated, deleteFile, destroyFileEntry bool) ([]*models.Image, error) {
	if err := i.LoadFiles(ctx, s.Repository); err != nil {
		return nil, err
	}

	var imgsDestroyed []*models.Image

	destroyer := &file.ZipDestroyer{
		FileDestroyer:   s.File,
		FolderDestroyer: s.Folder,
	}

	// for zip-based galleries, delete the images as well first
	for _, f := range i.Files.List() {
		// only do this where there are no other galleries related to the file
		otherGalleries, err := s.Repository.FindByFileID(ctx, f.Base().ID)
		if err != nil {
			return nil, err
		}

		if len(otherGalleries) > 1 {
			// other gallery associated, don't remove
			continue
		}

		thisDestroyed, err := s.ImageService.DestroyZipImages(ctx, f, fileDeleter, deleteGenerated)
		if err != nil {
			return nil, err
		}

		imgsDestroyed = append(imgsDestroyed, thisDestroyed...)

		if deleteFile {
			if err := destroyer.DestroyZip(ctx, f, fileDeleter.Deleter, deleteFile); err != nil {
				return nil, err
			}
		} else if destroyFileEntry {
			// destroy file DB entry without deleting filesystem file
			const deleteFileFromFS = false
			if err := destroyer.DestroyZip(ctx, f, nil, deleteFileFromFS); err != nil {
				return nil, err
			}
		}
	}

	return imgsDestroyed, nil
}

// PrepareZipRewrites removes queued zip entries and deletes galleries for empty zips.
func (s *Service) PrepareZipRewrites(ctx context.Context, fileDeleter *image.FileDeleter, deleteGenerated bool) error {
	return fileDeleter.PrepareZipRewrites(ctx, s.File, func(ctx context.Context, f models.File) error {
		galleries, err := s.Repository.FindByFileID(ctx, f.Base().ID)
		if err != nil {
			return err
		}

		// Collect folder galleries before DestroyZip clears their folder links.
		folders, err := s.Folder.FindByZipFileID(ctx, f.Base().ID)
		if err != nil {
			return err
		}
		for _, folder := range folders {
			folderGalleries, err := s.Repository.FindByFolderID(ctx, folder.ID)
			if err != nil {
				return err
			}
			galleries = append(galleries, folderGalleries...)
		}

		// The database can still list images whose files are already absent from the zip.
		if _, err := s.ImageService.DestroyZipImages(ctx, f, fileDeleter, deleteGenerated); err != nil {
			return err
		}

		destroyed := make(map[int]bool)
		for _, g := range galleries {
			if destroyed[g.ID] {
				continue
			}
			if err := g.LoadFiles(ctx, s.Repository); err != nil {
				return err
			}
			hasOtherFile := false
			for _, gf := range g.Files.List() {
				if gf.Base().ID != f.Base().ID {
					hasOtherFile = true
					break
				}
			}
			if hasOtherFile {
				continue
			}
			if err := s.Repository.Destroy(ctx, g.ID); err != nil {
				return err
			}
			destroyed[g.ID] = true
		}

		destroyer := &file.ZipDestroyer{FileDestroyer: s.File, FolderDestroyer: s.Folder}
		// PrepareZipRewrites has already set the original zip aside.
		return destroyer.DestroyZip(ctx, f, nil, false)
	})
}
