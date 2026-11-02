package task

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestUnzipFFmpegBtbNLayout(t *testing.T) {
	// Model the release-branch ZIP layout without downloading the archive.
	const root = "ffmpeg-n9.0-latest-win64-gpl-9.0/"
	files := map[string]string{
		root + "bin/ffmpeg.exe":  "ffmpeg binary",
		root + "bin/ffprobe.exe": "ffprobe binary",
		root + "bin/ffplay.exe":  "ffplay binary",
		root + "LICENSE.txt":     "license",
	}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(t.TempDir(), "ffmpeg.zip")
	if err := os.WriteFile(src, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	job := DownloadFFmpegJob{ConfigDirectory: dest}
	if err := job.unzip(src); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"ffmpeg.exe", "ffprobe.exe"} {
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil {
			t.Fatal(err)
		}
		if want := files[root+"bin/"+name]; string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("extracted %d entries, want only ffmpeg.exe and ffprobe.exe", len(entries))
	}
}
