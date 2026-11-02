package task

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/fsutil"
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

func TestDownloadSingleRemovesArchive(t *testing.T) {
	files := map[string]string{
		fsutil.GetExeName("ffmpeg"):  "ffmpeg binary",
		fsutil.GetExeName("ffprobe"): "ffprobe binary",
	}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, content := range files {
		entry, err := writer.Create("bin/" + name)
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

	for _, tt := range []struct {
		name        string
		status      int
		contentType string
		body        string
		wantError   string
	}{
		{
			name:        "success",
			status:      http.StatusOK,
			contentType: "application/zip",
			body:        archive.String(),
		},
		{
			name:      "HTTP error",
			status:    http.StatusInternalServerError,
			wantError: "failed to download ffmpeg",
		},
		{
			name:        "not a zip",
			status:      http.StatusOK,
			contentType: "text/plain",
			body:        "not a zip archive",
			wantError:   "downloaded file is not a zip archive",
		},
		{
			name:        "invalid zip",
			status:      http.StatusOK,
			contentType: "application/zip",
			body:        "invalid zip archive",
			wantError:   "failed to unzip ffmpeg archive",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			dest := t.TempDir()
			url := server.URL + "/ffmpeg.zip"
			job := DownloadFFmpegJob{ConfigDirectory: dest, urls: []string{url}}
			err := job.downloadSingle(context.Background(), url, nil)
			if tt.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("downloadSingle error = %v, want %q", err, tt.wantError)
			}

			if _, err := os.Stat(filepath.Join(dest, "ffmpeg.zip")); !os.IsNotExist(err) {
				t.Errorf("archive still exists or could not be checked: %v", err)
			}
			if tt.wantError == "" {
				for name, want := range files {
					got, err := os.ReadFile(filepath.Join(dest, name))
					if err != nil {
						t.Fatal(err)
					}
					if string(got) != want {
						t.Errorf("%s = %q, want %q", name, got, want)
					}
				}
			}
		})
	}
}
