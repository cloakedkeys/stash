package fsutil

import "path/filepath"

// IsSynologyMetadata reports whether path is in a Synology @eaDir metadata tree.
func IsSynologyMetadata(path string) bool {
	for path = filepath.Clean(path); ; {
		if filepath.Base(path) == "@eaDir" {
			return true
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
}
