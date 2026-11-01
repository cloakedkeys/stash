package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/file"
)

func TestSynologyScanAndVerifyFilters(t *testing.T) {
	config.InitializeEmpty()
	previous := instance
	instance = &Manager{Config: config.GetInstance()}
	t.Cleanup(func() { instance = previous })

	root := t.TempDir()
	filter := scanFilter{
		stashPaths:        config.StashConfigs{{Path: root}},
		generatedPath:     filepath.Join(root, "generated"),
		stashIgnoreFilter: file.NewStashIgnoreFilter(),
	}
	verify := verifyFilter{scanFilter: filter}

	for _, tt := range []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"@eaDir", true, false},
		{filepath.Join("@eaDir", "A.mp4"), true, false},
		{filepath.Join("@eaDir", "B.mp4"), true, false},
		{filepath.Join("@eaDir", "A.mp4", "thumb.jpg"), false, false},
		{filepath.Join("nested", "@eaDir", "thumb.jpg"), false, false},
		{"ordinary", true, true},
		{"@eaDirectory", true, true},
		{"A.mp4", false, true},
		{filepath.Join("prefix@eaDir", "B.mp4"), false, true},
	} {
		t.Run(tt.path, func(t *testing.T) {
			path := filepath.Join(root, tt.path)
			dir := filepath.Dir(path)
			if tt.isDir {
				dir = path
			}
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if !tt.isDir {
				if err := os.WriteFile(path, []byte("media"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := filter.Accept(context.Background(), path, info, ""); got != tt.want {
				t.Errorf("scan accepted = %v, want %v", got, tt.want)
			}
			if got := verify.Accept(context.Background(), path, info, ""); got != tt.want {
				t.Errorf("verify accepted = %v, want %v", got, tt.want)
			}
			archive := filepath.Join(root, "@eaDir", "gallery.zip")
			if filter.Accept(context.Background(), path, info, archive) {
				t.Error("scan accepted entry from archive in Synology metadata")
			}
			if verify.Accept(context.Background(), path, info, archive) {
				t.Error("verify accepted entry from archive in Synology metadata")
			}
		})
	}
}
