package scene

import (
	"context"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/file/video"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/scene/embedded"
)

// FileDeleter is an extension of file.Deleter that handles deletion of scene files.
type FileDeleter struct {
	*file.Deleter

	FileNamingAlgo models.HashAlgorithm
	Paths          *paths.Paths
}

// MarkGeneratedFiles marks for deletion the generated files for the provided scene.
// Generated files bypass trash and are permanently deleted since they can be regenerated.
func (d *FileDeleter) MarkGeneratedFiles(scene *models.Scene) error {
	if err := d.markSceneGeneratedFiles(scene); err != nil {
		return err
	}
	sceneHash := GeneratedKey(scene, d.FileNamingAlgo)

	if sceneHash == "" {
		return nil
	}

	// the markers' legacy folder
	markersFolder := d.Paths.SceneMarkers.GetFolderPath(sceneHash)

	exists, _ := fsutil.DirExists(markersFolder)
	if exists {
		if err := d.DirsWithoutTrash([]string{markersFolder}); err != nil {
			return err
		}
	}

	// the markers' files in the hash layout
	files := d.Paths.SceneMarkers.ListMarkerFiles(sceneHash)

	for _, f := range d.Paths.Scene.GeneratedFiles(sceneHash) {
		files = append(files, existingFiles(f)...)
	}
	return d.FilesWithoutTrash(files)
}

// markSceneGeneratedFiles marks the generated files named after the scene, rather than its file.
func (d *FileDeleter) markSceneGeneratedFiles(scene *models.Scene) error {
	subtitleDir := d.Paths.Scene.GetEmbeddedSubtitleDir(scene.ID)
	return embedded.MarkForDeletion(subtitleDir, func() error {
		if exists, _ := fsutil.DirExists(subtitleDir); exists {
			return d.DirsWithoutTrash([]string{subtitleDir})
		}
		return nil
	})
}

// MarkMarkerFiles deletes generated files for a scene marker with the
// provided scene and timestamp.
// Generated files bypass trash and are permanently deleted since they can be regenerated.
func (d *FileDeleter) MarkMarkerFiles(scene *models.Scene, seconds int) error {
	var files []string
	for _, f := range d.Paths.SceneMarkers.MarkerFiles(GeneratedKey(scene, d.FileNamingAlgo), seconds) {
		files = append(files, existingFiles(f)...)
	}
	return d.FilesWithoutTrash(files)
}

// Destroy deletes a scene and its associated relationships from the
// database.
func (s *Service) Destroy(ctx context.Context, scene *models.Scene, fileDeleter *FileDeleter, deleteGenerated, deleteFile, destroyFileEntry bool) error {
	// generated files are named after the file, so they stay while another scene uses it
	shared, err := s.sharesPrimaryFile(ctx, scene)
	if err != nil {
		return err
	}

	mqb := s.MarkerRepository
	markers, err := mqb.FindBySceneID(ctx, scene.ID)
	if err != nil {
		return err
	}

	for _, m := range markers {
		if err := s.DestroyMarker(ctx, scene, m, fileDeleter); err != nil {
			return err
		}
	}

	if deleteFile {
		if err := s.deleteFiles(ctx, scene, fileDeleter); err != nil {
			return err
		}
	} else if destroyFileEntry {
		if err := s.destroyFileEntries(ctx, scene); err != nil {
			return err
		}
	}

	if deleteGenerated {
		if shared {
			if err := fileDeleter.markSceneGeneratedFiles(scene); err != nil {
				return err
			}
		} else if err := fileDeleter.MarkGeneratedFiles(scene); err != nil {
			return err
		}
	}

	if err := s.Repository.Destroy(ctx, scene.ID); err != nil {
		return err
	}

	return nil
}

// deleteFiles deletes files from the database and file system
func (s *Service) deleteFiles(ctx context.Context, scene *models.Scene, fileDeleter *FileDeleter) error {
	if err := scene.LoadFiles(ctx, s.Repository); err != nil {
		return err
	}

	for _, f := range scene.Files.List() {
		// only delete files where there is no other associated scene
		otherScenes, err := s.Repository.FindByFileID(allScenes(ctx), f.ID)
		if err != nil {
			return err
		}

		if len(otherScenes) > 1 {
			// other scenes associated, don't remove
			continue
		}

		const deleteFile = true
		logger.Info("Deleting scene file: ", f.Path)
		if err := file.Destroy(ctx, s.File, f, fileDeleter.Deleter, deleteFile); err != nil {
			return err
		}

		// don't delete files in zip archives
		if f.ZipFileID == nil {
			funscriptPath := video.GetFunscriptPath(f.Path)
			funscriptExists, _ := fsutil.FileExists(funscriptPath)
			if funscriptExists {
				if err := fileDeleter.Files([]string{funscriptPath}); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// destroyFileEntries destroys file entries from the database without deleting
// the files from the filesystem
func (s *Service) destroyFileEntries(ctx context.Context, scene *models.Scene) error {
	if err := scene.LoadFiles(ctx, s.Repository); err != nil {
		return err
	}

	for _, f := range scene.Files.List() {
		// only destroy file entries where there is no other associated scene
		otherScenes, err := s.Repository.FindByFileID(allScenes(ctx), f.ID)
		if err != nil {
			return err
		}

		if len(otherScenes) > 1 {
			// other scenes associated, don't remove
			continue
		}

		const deleteFile = false
		logger.Info("Destroying scene file entry: ", f.Path)
		if err := file.Destroy(ctx, s.File, f, nil, deleteFile); err != nil {
			return err
		}
	}

	return nil
}

// DestroyMarker deletes the scene marker from the database and marks its generated
// files for deletion after the transaction is successfully committed.
func (s *Service) DestroyMarker(ctx context.Context, scene *models.Scene, sceneMarker *models.SceneMarker, fileDeleter *FileDeleter) error {
	if err := s.MarkerRepository.Destroy(ctx, sceneMarker.ID); err != nil {
		return err
	}

	return s.MarkMarkerFiles(ctx, scene, sceneMarker, fileDeleter)
}

// MarkMarkerFiles marks for deletion the generated files of the marker, at the second
// it was at on the scene, unless another marker uses them.
func (s *Service) MarkMarkerFiles(ctx context.Context, scene *models.Scene, sceneMarker *models.SceneMarker, fileDeleter *FileDeleter) error {
	seconds := int(sceneMarker.Seconds)
	inUse, err := s.markerFilesInUse(ctx, scene, seconds, sceneMarker.ID)
	if err != nil {
		return err
	}
	if inUse {
		return nil
	}

	return fileDeleter.MarkMarkerFiles(scene, seconds)
}

// existingFiles returns the paths of f that exist, in either layout.
func existingFiles(f paths.GeneratedFile) []string {
	var ret []string
	for _, p := range f.Both() {
		if exists, _ := fsutil.FileExists(p); exists {
			ret = append(ret, p)
		}
	}
	return ret
}
