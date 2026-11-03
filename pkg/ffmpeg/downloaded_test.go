package ffmpeg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveDownloadedFFmpeg(t *testing.T) {
	for _, names := range [][]string{
		nil,
		{getFFMpegFilename()},
		{getFFProbeFilename()},
		{getFFMpegFilename(), getFFProbeFilename()},
	} {
		t.Run(strings.Join(append([]string{"files"}, names...), "_"), func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range append([]string{"keep.txt"}, names...) {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("file"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := HasDownloadedFFmpeg(dir); got != (len(names) > 0) {
				t.Fatalf("HasDownloadedFFmpeg = %v, files = %v", got, names)
			}
			removed, err := RemoveDownloadedFFmpeg(dir)
			if err != nil || removed != (len(names) > 0) {
				t.Fatalf("RemoveDownloadedFFmpeg = %v, %v", removed, err)
			}
			if HasDownloadedFFmpeg(dir) {
				t.Fatal("downloaded files remain")
			}
			if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
				t.Fatal("unrelated file was removed:", err)
			}
			if removed, err := RemoveDownloadedFFmpeg(dir); removed || err != nil {
				t.Fatalf("repeat removal = %v, %v", removed, err)
			}
		})
	}
}

func TestRemoveDownloadedFFmpegLeavesOtherInstallations(t *testing.T) {
	dir := t.TempDir()
	external := t.TempDir()
	t.Setenv("PATH", external)
	for _, name := range []string{getFFMpegFilename(), getFFProbeFilename()} {
		if err := os.WriteFile(filepath.Join(external, name), []byte("external"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if HasDownloadedFFmpeg(dir) {
		t.Fatal("PATH installation counted as a download")
	}
	if removed, err := RemoveDownloadedFFmpeg(dir); removed || err != nil {
		t.Fatalf("removal = %v, %v", removed, err)
	}
	for _, name := range []string{getFFMpegFilename(), getFFProbeFilename()} {
		if _, err := os.Stat(filepath.Join(external, name)); err != nil {
			t.Fatal("external installation was removed:", err)
		}
	}
}

func TestRemoveDownloadedFFmpegRejectsDirectories(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, getFFMpegFilename())
	if err := os.Mkdir(filename, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveDownloadedFFmpeg(dir); err == nil {
		t.Fatal("expected an error for a directory")
	}
	if _, err := os.Stat(filename); err != nil {
		t.Fatal("directory was removed:", err)
	}
}

func TestIsDownloadedFile(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"", false},
		{filepath.Join(dir, getFFMpegFilename()), true},
		{filepath.Join(dir, getFFProbeFilename()), true},
		{filepath.Join(dir, ".", getFFMpegFilename()), true},
		{filepath.Join(other, getFFMpegFilename()), false},
		{filepath.Join(dir, "keep.txt"), false},
	} {
		if got := IsDownloadedFile(dir, tc.path); got != tc.want {
			t.Errorf("IsDownloadedFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// A config path such as ./config/ffmpeg is relative to the working directory.
func TestIsDownloadedFileRelativePath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	configDir := filepath.Join(dir, "config")
	for _, tc := range []struct {
		path string
		want bool
	}{
		{filepath.Join(".", "config", getFFMpegFilename()), true},
		{filepath.Join("config", getFFProbeFilename()), true},
		{getFFMpegFilename(), false},
	} {
		if got := IsDownloadedFile(configDir, tc.path); got != tc.want {
			t.Errorf("IsDownloadedFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
