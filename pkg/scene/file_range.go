package scene

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/stashapp/stash/pkg/models"
)

// A scene can be part of a file, from a start time to an end time, in seconds. Several
// scenes with a range can share a file, and so can one scene that uses the whole of it.
// A scene with a range has exactly one file, its primary file.

var (
	ErrRangeNeedsOneFile = errors.New("a scene with a start or end time must have exactly one file")
	ErrWholeFileInUse    = errors.New("another scene already uses the whole of this file")
	ErrMergeSharedFile   = errors.New("scenes with a start or end time, or that share a file with another scene, can't be merged yet")
)

// HasRange reports whether the scene is part of its file rather than the whole of it.
func HasRange(s *models.Scene) bool {
	return s.StartTime != nil || s.EndTime != nil
}

// usesWholeFile reports whether the scene uses the whole of the file.
func usesWholeFile(s *models.Scene, fileID models.FileID) bool {
	return s.PrimaryFileID == nil || *s.PrimaryFileID != fileID || !HasRange(s)
}

// ValidateRange checks a start and end time against the file they are in.
func ValidateRange(f *models.VideoFile, start, end *float64) error {
	for _, v := range []*float64{start, end} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0)) {
			return errors.New("start and end times must be numbers of seconds")
		}
	}

	if start != nil && *start < 0 {
		return errors.New("start time can't be before the start of the file")
	}
	if end != nil && *end <= 0 {
		return errors.New("end time must be after the start of the file")
	}
	if start != nil && end != nil && *start >= *end {
		return errors.New("start time must be before the end time")
	}

	// a file that hasn't been read properly has no duration, so its length can't be checked
	duration := f.Duration
	if duration > 0 && !math.IsInf(duration, 0) && !math.IsNaN(duration) {
		if start != nil && *start >= duration {
			return fmt.Errorf("start time is after the end of the file, at %g seconds", duration)
		}
		if end != nil && *end > duration {
			return fmt.Errorf("end time is after the end of the file, at %g seconds", duration)
		}
	}

	return nil
}

// otherScenes returns the scenes other than sceneID that use the file.
func (s *Service) otherScenes(ctx context.Context, sceneID int, fileID models.FileID) ([]*models.Scene, error) {
	scenes, err := s.Repository.FindByFileID(ctx, fileID)
	if err != nil {
		return nil, err
	}

	var ret []*models.Scene
	for _, o := range scenes {
		if o.ID != sceneID {
			ret = append(ret, o)
		}
	}

	return ret, nil
}

func (s *Service) wholeFileInUse(ctx context.Context, sceneID int, fileID models.FileID) (bool, error) {
	others, err := s.otherScenes(ctx, sceneID, fileID)
	if err != nil {
		return false, err
	}

	for _, o := range others {
		if usesWholeFile(o, fileID) {
			return true, nil
		}
	}

	return false, nil
}

func (s *Service) findVideoFile(ctx context.Context, fileID models.FileID) (*models.VideoFile, error) {
	f, err := s.File.Find(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if len(f) == 0 {
		return nil, fmt.Errorf("file with id %d not found", fileID)
	}

	vf, ok := f[0].(*models.VideoFile)
	if !ok {
		return nil, fmt.Errorf("%s is not a video file", f[0].Base().Path)
	}

	return vf, nil
}

// ValidateRangeUpdate checks a change to where a scene starts and ends in its file.
// start and end are the values the scene will have after the change.
func (s *Service) ValidateRangeUpdate(ctx context.Context, scene *models.Scene, start, end *float64) error {
	if err := scene.LoadFiles(ctx, s.Repository); err != nil {
		return err
	}
	files := scene.Files.List()

	if start == nil && end == nil {
		// the scene will use the whole file, which only one scene can
		if HasRange(scene) && len(files) == 1 {
			inUse, err := s.wholeFileInUse(ctx, scene.ID, files[0].ID)
			if err != nil {
				return err
			}
			if inUse {
				return ErrWholeFileInUse
			}
		}
		return nil
	}

	if len(files) != 1 {
		return ErrRangeNeedsOneFile
	}

	return ValidateRange(files[0], start, end)
}

// checkMergeable refuses a merge involving a part of a file, or a file shared with
// a scene outside the merge, which moving the files would take from that scene.
func (s *Service) checkMergeable(ctx context.Context, dest *models.Scene, sources []*models.Scene) error {
	inMerge := map[int]bool{dest.ID: true}
	for _, src := range sources {
		inMerge[src.ID] = true
	}

	for _, sc := range append([]*models.Scene{dest}, sources...) {
		if HasRange(sc) {
			return ErrMergeSharedFile
		}
		if err := sc.LoadFiles(ctx, s.Repository); err != nil {
			return err
		}
		for _, f := range sc.Files.List() {
			others, err := s.Repository.FindByFileID(ctx, f.ID)
			if err != nil {
				return err
			}
			for _, o := range others {
				if !inMerge[o.ID] {
					return ErrMergeSharedFile
				}
			}
		}
	}

	return nil
}

// sharesPrimaryFile reports whether another scene uses the scene's primary file. Its
// generated files, such as previews and sprites, are named after the file, so they
// belong to all of those scenes.
func (s *Service) sharesPrimaryFile(ctx context.Context, scene *models.Scene) (bool, error) {
	if scene.PrimaryFileID == nil {
		return false, nil
	}

	others, err := s.otherScenes(ctx, scene.ID, *scene.PrimaryFileID)
	if err != nil {
		return false, err
	}

	return len(others) > 0, nil
}
