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
	ErrMergeRange        = errors.New("a merge can't set a start or end time")
)

// HasRange reports whether the scene is part of its file rather than the whole of it.
func HasRange(s *models.Scene) bool {
	return s.StartTime != nil || s.EndTime != nil
}

// usesWholeFile reports whether the scene uses the whole of the file.
func usesWholeFile(s *models.Scene, fileID models.FileID) bool {
	return s.PrimaryFileID == nil || *s.PrimaryFileID != fileID || !HasRange(s)
}

// allScenes returns a context that sees every scene. The rules about which scenes share
// a file hold across the whole library, whatever part of it the account can see.
func allScenes(ctx context.Context) context.Context {
	return models.WithLibraryLimit(ctx, nil)
}

// fileLocker is a store that can hold a file until the transaction ends, so that two
// changes to which scenes use it run one after the other. PostgreSQL needs this. SQLite
// runs one write at a time anyway.
type fileLocker interface {
	LockFile(ctx context.Context, fileID models.FileID) error
}

func (s *Service) lockFile(ctx context.Context, fileID models.FileID) error {
	if l, ok := s.Repository.(fileLocker); ok {
		return l.LockFile(ctx, fileID)
	}
	return nil
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
	if knownDuration(f) {
		if start != nil && *start >= f.Duration {
			return fmt.Errorf("start time is after the end of the file, at %g seconds", f.Duration)
		}
		if end != nil && *end > f.Duration {
			return fmt.Errorf("end time is after the end of the file, at %g seconds", f.Duration)
		}
	}

	return nil
}

func knownDuration(f *models.VideoFile) bool {
	return f.Duration > 0 && !math.IsInf(f.Duration, 0) && !math.IsNaN(f.Duration)
}

// normaliseRange drops a start at the start of the file and an end at its end, so that
// a range covering the whole file is the whole file. The range must be valid.
func normaliseRange(f *models.VideoFile, start, end *float64) (*float64, *float64) {
	if start != nil && *start == 0 {
		start = nil
	}
	if end != nil && knownDuration(f) && *end >= f.Duration {
		end = nil
	}
	return start, end
}

// checkRange validates and normalises a range in a file.
func checkRange(f *models.VideoFile, start, end *float64) (*float64, *float64, error) {
	if err := ValidateRange(f, start, end); err != nil {
		return nil, nil, err
	}
	start, end = normaliseRange(f, start, end)
	return start, end, nil
}

// otherScenes returns the scenes other than sceneID that use the file.
func (s *Service) otherScenes(ctx context.Context, sceneID int, fileID models.FileID) ([]*models.Scene, error) {
	scenes, err := s.Repository.FindByFileID(allScenes(ctx), fileID)
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
	if err := s.lockFile(ctx, fileID); err != nil {
		return false, err
	}

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
// start and end are the values the scene will have after the change. It returns the
// values to store, which are nil where they are the start or end of the file.
func (s *Service) ValidateRangeUpdate(ctx context.Context, scene *models.Scene, start, end *float64) (*float64, *float64, error) {
	if err := scene.LoadFiles(ctx, s.Repository); err != nil {
		return nil, nil, err
	}
	files := scene.Files.List()

	if start != nil || end != nil {
		if len(files) != 1 {
			return nil, nil, ErrRangeNeedsOneFile
		}

		var err error
		start, end, err = checkRange(files[0], start, end)
		if err != nil {
			return nil, nil, err
		}
	}

	if start == nil && end == nil && HasRange(scene) && len(files) == 1 {
		// the scene will use the whole file, which only one scene can
		inUse, err := s.wholeFileInUse(ctx, scene.ID, files[0].ID)
		if err != nil {
			return nil, nil, err
		}
		if inUse {
			return nil, nil, ErrWholeFileInUse
		}
	}

	return start, end, nil
}

// checkMergeable refuses a merge involving a part of a file, or a file shared with
// a scene outside the merge, which moving the files would take from that scene.
func (s *Service) checkMergeable(ctx context.Context, dest *models.Scene, sources []*models.Scene, values models.ScenePartial) error {
	if values.StartTime.Set || values.EndTime.Set {
		return ErrMergeRange
	}

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
			others, err := s.Repository.FindByFileID(allScenes(ctx), f.ID)
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

// markerFilesInUse reports whether a marker other than markerID uses the generated files
// of a marker at the second given on the scene. They are named after the scene's primary
// file and the second, so the scenes sharing that file share them.
func (s *Service) markerFilesInUse(ctx context.Context, scene *models.Scene, seconds int, markerID int) (bool, error) {
	scenes := []*models.Scene{scene}
	if scene.PrimaryFileID != nil {
		others, err := s.otherScenes(ctx, scene.ID, *scene.PrimaryFileID)
		if err != nil {
			return false, err
		}
		for _, o := range others {
			if o.PrimaryFileID != nil && *o.PrimaryFileID == *scene.PrimaryFileID {
				scenes = append(scenes, o)
			}
		}
	}

	for _, sc := range scenes {
		markers, err := s.MarkerRepository.FindBySceneID(allScenes(ctx), sc.ID)
		if err != nil {
			return false, err
		}
		for _, m := range markers {
			if m.ID != markerID && int(m.Seconds) == seconds {
				return true, nil
			}
		}
	}

	return false, nil
}
