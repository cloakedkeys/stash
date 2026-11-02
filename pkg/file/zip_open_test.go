package file

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/models"
)

func TestZipFSOpenOnlyClosesArchive(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entry   string
		wantErr error
	}{
		{name: "missing", entry: "missing.txt", wantErr: fs.ErrNotExist},
		{name: "unsupported compression", entry: "unsupported.txt", wantErr: zip.ErrAlgorithm},
		{name: "success", entry: "present.txt"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			zipPath := filepath.Join(t.TempDir(), "archive.zip")
			archive, err := os.Create(zipPath)
			if err != nil {
				t.Fatal(err)
			}
			defer archive.Close()
			writer := zip.NewWriter(archive)
			entry, err := writer.Create("present.txt")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(entry, "content"); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.CreateRaw(&zip.FileHeader{Name: "unsupported.txt", Method: 99}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(zipPath)
			if err != nil {
				t.Fatal(err)
			}
			zfs, err := newZipFS(&OsFS{}, zipPath, info.Size())
			if err != nil {
				t.Fatal(err)
			}
			// close it when the test ends, even on the old code
			defer zfs.Close()
			reader, err := zfs.OpenOnly(filepath.Join(zipPath, tt.entry))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("OpenOnly error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if reader != nil {
					reader.Close()
					t.Fatal("OpenOnly returned a reader on failure")
				}
			} else {
				data, err := io.ReadAll(reader)
				if err != nil || string(data) != "content" {
					t.Fatalf("read = %q, %v; want content", data, err)
				}
				if err := reader.Close(); err != nil {
					t.Fatal(err)
				}
			}
			// the zip is closed: Windows refuses to rename a file that is still open
			if err := os.Rename(zipPath, zipPath+".renamed"); err != nil {
				t.Fatalf("rename archive after OpenOnly: %v", err)
			}
		})
	}
}

func TestBaseFileOpenMissingZipEntryReleasesArchive(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "archive.zip")
	makeTestZip(t, zipPath, "", map[string]string{"present.txt": "content"})
	info, err := os.Stat(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	file := &models.BaseFile{
		Path: filepath.Join(zipPath, "missing.txt"),
		DirEntry: models.DirEntry{
			ZipFile: &models.BaseFile{Path: zipPath, Size: info.Size()},
		},
	}
	reader, err := file.Open(&OsFS{})
	if reader != nil {
		reader.Close()
		t.Fatal("Open returned a reader for a missing entry")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open error = %v, want fs.ErrNotExist", err)
	}
	// Windows refuses to rename an archive while its file handle is open.
	if err := os.Rename(zipPath, zipPath+".renamed"); err != nil {
		t.Fatalf("rename archive after failed Open: %v", err)
	}
}
