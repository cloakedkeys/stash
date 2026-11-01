package api

import (
	"runtime"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
)

func TestIsStashPath(t *testing.T) {
	type testCase struct {
		name    string
		path    string
		library string
		want    bool
	}
	tests := []testCase{
		{"exact match", "/data", "/data", true},
		{"library trailing slash", "/data", "/data/", true},
		{"gallery trailing slash", "/data/", "/data", true},
		{"clean both paths", "/data/./", "/data/child/../", true},
		{"root", "/", "/", true},
		{"child folder", "/data/gallery", "/data/", false},
		{"parent folder", "/data", "/data/gallery/", false},
		{"shared prefix", "/database", "/data/", false},
		{"different folder", "/other", "/data/", false},
	}
	if runtime.GOOS == "windows" {
		tests = append(tests, []testCase{
			{"windows library forward slashes", `C:\data`, "C:/data/", true},
			{"windows gallery forward slashes", "C:/data/", `C:\data\`, true},
			{"windows drive root", `C:\`, "C:/", true},
			{"windows UNC", `\\server\share\data`, "//server/share/data/", true},
		}...)
	} else {
		// A backslash is a filename character on Unix, not a separator.
		tests = append(tests, testCase{"literal backslash", `/data\`, "/data/", false})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stashes := config.StashConfigs{{Path: "/unrelated"}, {Path: tt.library}}
			if got := isStashPath(tt.path, stashes); got != tt.want {
				t.Errorf("isStashPath(%q, %q) = %v, want %v", tt.path, tt.library, got, tt.want)
			}
		})
	}
	t.Run("no libraries", func(t *testing.T) {
		if isStashPath("/data", nil) {
			t.Fatal("path matched an empty library list")
		}
	})
}
