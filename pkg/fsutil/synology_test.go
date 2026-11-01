package fsutil

import (
	"path/filepath"
	"testing"
)

func TestIsSynologyMetadata(t *testing.T) {
	for _, tt := range []struct {
		path string
		want bool
	}{
		{"@eaDir", true},
		{filepath.Join("content", "@eaDir"), true},
		{filepath.Join("content", "@eaDir", "A.mp4", "SYNOINDEX_MEDIA_INFO"), true},
		{filepath.Join("content", "nested", "@eaDir", "thumb.jpg"), true},
		{filepath.Join("content", "@eaDirectory", "A.mp4"), false},
		{filepath.Join("content", "prefix@eaDir", "A.mp4"), false},
		{filepath.Join("content", "@eaDir.mp4"), false},
		{filepath.Join("content", "A.mp4"), false},
		{".", false},
		{string(filepath.Separator), false},
	} {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsSynologyMetadata(tt.path); got != tt.want {
				t.Errorf("IsSynologyMetadata(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
