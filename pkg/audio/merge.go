package audio

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sliceutil"
)

type MergeOptions struct {
	AudioPartial       models.AudioPartial
	IncludePlayHistory bool
	IncludeOHistory    bool
}

func (s *Service) Merge(ctx context.Context, sourceIDs []int, destinationID int, fileDeleter *FileDeleter, options MergeOptions) error {
	audioPartial := options.AudioPartial

	// ensure source ids are unique
	sourceIDs = sliceutil.AppendUniques(nil, sourceIDs)

	// ensure destination is not in source list
	if slices.Contains(sourceIDs, destinationID) {
		return errors.New("destination audio cannot be in source list")
	}

	dest, err := s.Repository.Find(ctx, destinationID)
	if err != nil {
		return fmt.Errorf("finding destination audio ID %d: %w", destinationID, err)
	}

	sources, err := s.Repository.FindMany(ctx, sourceIDs)
	if err != nil {
		return fmt.Errorf("finding source audios: %w", err)
	}

	var fileIDs []models.FileID

	for _, src := range sources {
		if err := src.LoadRelationships(ctx, s.Repository); err != nil {
			return fmt.Errorf("loading audio relationships from %d: %w", src.ID, err)
		}

		for _, f := range src.Files.List() {
			fileIDs = append(fileIDs, f.Base().ID)
		}
	}

	// move files to destination audio
	if len(fileIDs) > 0 {
		if err := s.Repository.AssignFiles(ctx, destinationID, fileIDs); err != nil {
			return fmt.Errorf("moving files to destination audio: %w", err)
		}

		// if audio didn't already have a primary file, then set it now
		if dest.PrimaryFileID == nil {
			audioPartial.PrimaryFileID = &fileIDs[0]
		} else {
			// don't allow changing primary file ID from the input values
			audioPartial.PrimaryFileID = nil
		}
	}

	if _, err := s.Repository.UpdatePartial(ctx, destinationID, audioPartial); err != nil {
		return fmt.Errorf("updating audio: %w", err)
	}

	if err := s.Repository.MergeUserData(ctx, destinationID, sourceIDs, options.IncludePlayHistory, options.IncludeOHistory); err != nil {
		return fmt.Errorf("merging audio user data: %w", err)
	}

	// delete old audios
	for _, src := range sources {
		const deleteFile = false
		const destroyFileEntry = false
		if err := s.Destroy(ctx, src, fileDeleter, deleteFile, destroyFileEntry); err != nil {
			return fmt.Errorf("deleting audio %d: %w", src.ID, err)
		}
	}

	return nil
}
