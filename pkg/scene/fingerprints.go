package scene

import (
	"context"
	"fmt"

	"github.com/stashapp/stash/pkg/models"
)

// Fingerprints returns the fingerprints that identify the scene: those of its files.
// A scene that's part of a file has none, since its file's fingerprints are those of
// the whole of it. The scene's files must be loaded.
func Fingerprints(s *models.Scene) models.Fingerprints {
	if HasRange(s) {
		return nil
	}

	var ret models.Fingerprints
	for _, f := range s.Files.List() {
		ret = append(ret, f.Fingerprints...)
	}

	return ret
}

// GetFingerprints returns the fingerprints for the given scene ids.
func (s *Service) GetScenesFingerprints(ctx context.Context, ids []int) ([]models.Fingerprints, error) {
	fingerprints := make([]models.Fingerprints, len(ids))

	qb := s.Repository

	for i, sceneID := range ids {
		scene, err := qb.Find(ctx, sceneID)
		if err != nil {
			return nil, err
		}

		if scene == nil {
			return nil, fmt.Errorf("scene with id %d not found", sceneID)
		}

		if err := scene.LoadFiles(ctx, qb); err != nil {
			return nil, err
		}

		fingerprints[i] = Fingerprints(scene)
	}

	return fingerprints, nil
}
