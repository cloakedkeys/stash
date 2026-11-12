package scene

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sliceutil"
	"github.com/stashapp/stash/pkg/txn"
)

type MergeOptions struct {
	ScenePartial       models.ScenePartial
	IncludePlayHistory bool
	IncludeOHistory    bool
}

func (s *Service) Merge(ctx context.Context, sourceIDs []int, destinationID int, fileDeleter *FileDeleter, options MergeOptions) error {
	scenePartial := options.ScenePartial

	// ensure source ids are unique
	sourceIDs = sliceutil.AppendUniques(nil, sourceIDs)

	// ensure destination is not in source list
	if slices.Contains(sourceIDs, destinationID) {
		return errors.New("destination scene cannot be in source list")
	}

	dest, err := s.Repository.Find(ctx, destinationID)
	if err != nil {
		return fmt.Errorf("finding destination scene ID %d: %w", destinationID, err)
	}

	sources, err := s.Repository.FindMany(ctx, sourceIDs)
	if err != nil {
		return fmt.Errorf("finding source scenes: %w", err)
	}

	if dest != nil {
		if err := s.checkMergeable(ctx, dest, sources, scenePartial); err != nil {
			return err
		}
	}

	var fileIDs []models.FileID
	var movedMarkers []movedMarker

	for _, src := range sources {
		if err := src.LoadRelationships(ctx, s.Repository); err != nil {
			return fmt.Errorf("loading scene relationships from %d: %w", src.ID, err)
		}

		for _, f := range src.Files.List() {
			fileIDs = append(fileIDs, f.Base().ID)
		}

		moved, err := s.mergeSceneMarkers(ctx, destinationID, src)
		if err != nil {
			return err
		}
		movedMarkers = append(movedMarkers, moved...)
	}

	// move files to destination scene
	if len(fileIDs) > 0 {
		if err := s.Repository.AssignFiles(ctx, destinationID, fileIDs); err != nil {
			return fmt.Errorf("moving files to destination scene: %w", err)
		}

		// if scene didn't already have a primary file, then set it now
		if dest.PrimaryFileID == nil {
			scenePartial.PrimaryFileID = &fileIDs[0]
		} else {
			// don't allow changing primary file ID from the input values
			scenePartial.PrimaryFileID = nil
		}
	}

	// merge user data, including play and o history, for all users.
	// This is done before updating the destination, so that the provided values
	// take precedence for the current user.
	if err := s.Repository.MergeUserData(ctx, destinationID, sourceIDs, options.IncludePlayHistory, options.IncludeOHistory); err != nil {
		return fmt.Errorf("merging user data: %w", err)
	}

	updated, err := s.Repository.UpdatePartial(ctx, destinationID, scenePartial)
	if err != nil {
		return fmt.Errorf("updating scene: %w", err)
	}

	// The destination's hash is known now that it has its files. A
	// destination that had no file has taken the first source's.
	s.moveMarkerFiles(ctx, movedMarkers, GeneratedKey(updated, s.Config.GetVideoFileNamingAlgorithm()))

	// delete old scenes
	for _, src := range sources {
		const deleteGenerated = false
		const deleteFile = false
		const destroyFileEntry = false
		if err := s.Destroy(ctx, src, fileDeleter, deleteGenerated, deleteFile, destroyFileEntry); err != nil {
			return fmt.Errorf("deleting scene %d: %w", src.ID, err)
		}
		// Marker migration above runs after commit; keep the old files until then.
		if err := fileDeleter.markSceneGeneratedFiles(src); err != nil {
			return err
		}
		if err := InvalidateChangedGeneratedKey(ctx, s.Repository, s.Paths, s.Config.GetVideoFileNamingAlgorithm(), src, updated); err != nil {
			return err
		}
	}

	return nil
}

// movedMarker is a marker that has moved to another scene.
type movedMarker struct {
	srcHash string
	seconds int
}

// mergeSceneMarkers moves the source's markers to the destination scene.
func (s *Service) mergeSceneMarkers(ctx context.Context, destinationID int, src *models.Scene) ([]movedMarker, error) {
	markers, err := s.MarkerRepository.FindBySceneID(ctx, src.ID)
	if err != nil {
		return nil, fmt.Errorf("finding scene markers: %w", err)
	}

	srcHash := GeneratedKey(src, s.Config.GetVideoFileNamingAlgorithm())

	var ret []movedMarker
	for _, m := range markers {
		// updated the scene id
		m.SceneID = destinationID

		if err := s.MarkerRepository.Update(ctx, m); err != nil {
			return nil, fmt.Errorf("updating scene marker %d: %w", m.ID, err)
		}

		ret = append(ret, movedMarker{srcHash: srcHash, seconds: int(m.Seconds)})
	}

	return ret, nil
}

// moveMarkerFiles moves the generated files of the moved markers to destHash,
// after the commit.
//
// Merge defers deletion of obsolete source keys until after these moves.
func (s *Service) moveMarkerFiles(ctx context.Context, markers []movedMarker, destHash string) {
	type rename struct {
		src  string
		dest string
	}

	var toRename []rename

	for _, m := range markers {
		// a scene without a file has no hash, and so no files
		if m.srcHash == "" || destHash == "" || m.srcHash == destHash {
			continue
		}

		markerPaths := s.Paths.SceneMarkers
		toRename = append(toRename, []rename{
			{
				src:  markerPaths.GetScreenshotPath(m.srcHash, m.seconds),
				dest: markerPaths.GetScreenshotPath(destHash, m.seconds),
			},
			{
				src:  markerPaths.GetVideoPreviewPath(m.srcHash, m.seconds),
				dest: markerPaths.GetVideoPreviewPath(destHash, m.seconds),
			},
			{
				src:  markerPaths.GetWebpPreviewPath(m.srcHash, m.seconds),
				dest: markerPaths.GetWebpPreviewPath(destHash, m.seconds),
			},
		}...)
	}

	if len(toRename) == 0 {
		return
	}

	txn.AddPostCommitHook(ctx, func(ctx context.Context) {
		// rename the files if they exist
		for _, e := range toRename {
			srcExists, _ := fsutil.FileExists(e.src)
			destExists, _ := fsutil.FileExists(e.dest)

			if srcExists && !destExists {
				destDir := filepath.Dir(e.dest)
				// in the hash layout, the folder's parent may not exist either
				if err := fsutil.EnsureDirAll(destDir); err != nil {
					logger.Errorf("Error creating generated marker folder %s: %v", destDir, err)
					continue
				}

				if err := os.Rename(e.src, e.dest); err != nil {
					logger.Errorf("Error renaming generated marker file from %s to %s: %v", e.src, e.dest, err)
				}
			}
		}
	})
}
