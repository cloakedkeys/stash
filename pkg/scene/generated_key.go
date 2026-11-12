package scene

import "github.com/stashapp/stash/pkg/models"

// GeneratedKey returns the key used to name a scene's generated media, using
// the configured video file naming algorithm. For now, parts and whole-file
// scenes both keep the existing file hash, including an empty hash if missing.
// Range suffixes will be introduced separately; fingerprints remain file-level.
func GeneratedKey(s *models.Scene, hashAlgorithm models.HashAlgorithm) string {
	return s.GetHash(hashAlgorithm)
}
