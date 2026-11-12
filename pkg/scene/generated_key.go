package scene

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/txn"
)

// GeneratedKey names generated media by file and range, independently of scene ID.
// Whole-file scenes retain the file hash. Parts use <hash>_<startMs>-<endMs>,
// rounding seconds to the nearest integer millisecond. A nil start means zero;
// a nil end uses the primary file duration, or "end" when it is unknown (zero).
// Fingerprints and export identity remain file-level.
func GeneratedKey(s *models.Scene, hashAlgorithm models.HashAlgorithm) string {
	hash := s.GetHash(hashAlgorithm)
	if hash == "" || !HasRange(s) {
		return hash
	}
	start := float64(0)
	if s.StartTime != nil {
		start = *s.StartTime
	}
	duration := s.PrimaryFileDuration
	if s.Files.PrimaryLoaded() && s.Files.Primary() != nil {
		duration = s.Files.Primary().Duration
	}
	end := "end"
	if s.EndTime != nil {
		end = fmt.Sprintf("%.0f", math.Round(*s.EndTime*1000))
	} else if duration > 0 {
		end = fmt.Sprintf("%.0f", math.Round(duration*1000))
	}
	return fmt.Sprintf("%s_%.0f-%s", hash, math.Round(start*1000), end)
}

var generatedKeyPattern = regexp.MustCompile(`^(?:[0-9a-f]{16}|[0-9a-f]{32})(?:_[0-9]+-(?:[0-9]+|end))?$`)

// GeneratedKeyHash validates a generated key and returns its file fingerprint.
func GeneratedKeyHash(key string) (string, bool) {
	if !generatedKeyPattern.MatchString(key) {
		return "", false
	}
	hash, _, _ := strings.Cut(key, "_")
	return hash, true
}

// InvalidateChangedGeneratedKey schedules removal of an obsolete key after commit.
// Lookup runs without account limits so a hidden sibling keeps its shared media.
func InvalidateChangedGeneratedKey(ctx context.Context, finder models.SceneFinder, p *paths.Paths, algo models.HashAlgorithm, old, updated *models.Scene) error {
	if p == nil || old == nil || GeneratedKey(old, algo) == GeneratedKey(updated, algo) {
		return nil
	}
	key := GeneratedKey(old, algo)
	if key == "" {
		return nil
	}
	txn.AddPreCommitHook(ctx, func(ctx context.Context) error {
		scenes, err := scenesWithGeneratedKey(allScenes(ctx), finder, old.GetHash(algo), key, algo)
		if err != nil {
			return err
		}
		if len(scenes) == 0 {
			txn.AddPostCommitHook(ctx, func(context.Context) { InvalidateGeneratedFiles(p, key) })
		}
		return nil
	})
	return nil
}

func scenesWithGeneratedKey(ctx context.Context, finder models.SceneFinder, hash, key string, algo models.HashAlgorithm) ([]*models.Scene, error) {
	typ := models.FingerprintTypeOshash
	if algo == models.HashAlgorithmMd5 {
		typ = models.FingerprintTypeMD5
	}
	scenes, err := finder.FindByFingerprints(ctx, []models.Fingerprint{{Type: typ, Fingerprint: hash}})
	if err != nil {
		return nil, err
	}
	var ret []*models.Scene
	for _, s := range scenes {
		if GeneratedKey(s, algo) == key {
			ret = append(ret, s)
		}
	}
	return ret, nil
}

// InvalidateChangedGeneratedKey removes an obsolete range key after commit.
func (s *Service) InvalidateChangedGeneratedKey(ctx context.Context, old, updated *models.Scene) error {
	return InvalidateChangedGeneratedKey(ctx, s.Repository, s.Paths, s.Config.GetVideoFileNamingAlgorithm(), old, updated)
}
