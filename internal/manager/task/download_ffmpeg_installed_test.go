package task

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/fsutil"
)

func TestDownloadFFmpegAlreadyInstalled(t *testing.T) {
	for _, names := range [][]string{{"ffmpeg"}, {"ffprobe"}, {"ffmpeg", "ffprobe"}} {
		t.Run(names[0], func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range names {
				if err := os.WriteFile(filepath.Join(dir, fsutil.GetExeName(name)), []byte("installed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			// A cancelled context prevents network access on the old code. The
			// installed check must happen before any download is attempted.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			job := DownloadFFmpegJob{ConfigDirectory: dir}
			err := job.download(ctx, nil)
			if err == nil || err.Error() != "FFmpeg is already installed." {
				t.Fatalf("download error = %v, want a short already-installed message", err)
			}
			for _, name := range names {
				data, err := os.ReadFile(filepath.Join(dir, fsutil.GetExeName(name)))
				if err != nil || string(data) != "installed" {
					t.Fatalf("installed %s changed: %q, %v", name, data, err)
				}
			}
		})
	}
}
