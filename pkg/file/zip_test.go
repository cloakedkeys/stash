package file

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// makeTestZip creates a zip file at path containing the given entries (name → content).
// If comment is non-empty it is set as the archive-level comment.
func makeTestZip(t *testing.T, path, comment string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range entries {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("zip.Create %q: %v", name, err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %q: %v", name, err)
		}
	}
	if comment != "" {
		if err := w.SetComment(comment); err != nil {
			t.Fatalf("zip.SetComment: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
}

// readZipEntries returns a map of name → content from the zip at path.
func readZipEntries(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %q: %v", path, err)
	}
	defer r.Close()

	out := make(map[string]string, len(r.File))
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %q: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read entry %q: %v", f.Name, err)
		}
		out[f.Name] = string(data)
	}
	return out
}

func readZipComment(t *testing.T, path string) string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %q: %v", path, err)
	}
	defer r.Close()
	return r.Comment
}

func TestRemoveEntriesFromZip_Single(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{
		"keep.jpg":   "keep-data",
		"remove.jpg": "remove-data",
	})

	tmpPath, err := RemoveEntriesFromZip(zipPath, []string{"remove.jpg"})
	if err != nil {
		t.Fatalf("RemoveEntriesFromZip: %v", err)
	}
	defer os.Remove(tmpPath)

	got := readZipEntries(t, tmpPath)
	if _, present := got["remove.jpg"]; present {
		t.Error("remove.jpg should not be in output zip")
	}
	if got["keep.jpg"] != "keep-data" {
		t.Errorf("keep.jpg content = %q, want %q", got["keep.jpg"], "keep-data")
	}
}

func TestRemoveEntriesFromZip_Multiple(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{
		"a.jpg": "a",
		"b.jpg": "b",
		"c.jpg": "c",
	})

	tmpPath, err := RemoveEntriesFromZip(zipPath, []string{"a.jpg", "b.jpg"})
	if err != nil {
		t.Fatalf("RemoveEntriesFromZip: %v", err)
	}
	defer os.Remove(tmpPath)

	got := readZipEntries(t, tmpPath)
	for _, name := range []string{"a.jpg", "b.jpg"} {
		if _, present := got[name]; present {
			t.Errorf("%s should not be in output zip", name)
		}
	}
	if got["c.jpg"] != "c" {
		t.Errorf("c.jpg content = %q, want %q", got["c.jpg"], "c")
	}
}

func TestRemoveEntriesFromZip_EntryNotPresent(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{
		"a.jpg": "a",
		"b.jpg": "b",
	})

	// "ghost.jpg" is already gone, for example after the zip was edited outside stash
	tmpPath, err := RemoveEntriesFromZip(zipPath, []string{"a.jpg", "ghost.jpg"})
	if err != nil {
		t.Fatalf("RemoveEntriesFromZip: %v", err)
	}
	defer os.Remove(tmpPath)
	if got := readZipEntries(t, tmpPath); len(got) != 1 || got["b.jpg"] != "b" {
		t.Errorf("expected only b.jpg, got %v", got)
	}

	// with nothing to remove, no copy is written
	if _, err := RemoveEntriesFromZip(zipPath, []string{"ghost.jpg"}); !errors.Is(err, errNothingToRemove) {
		t.Fatalf("err = %v, want errNothingToRemove", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("expected only the zip and the first copy, got %d files", len(entries))
	}
}

// entries are matched by the names the scanner sees, which zip.Reader.Open cleans
func TestRemoveEntriesFromZip_MatchesScannerNames(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{
		`sub\windows.jpg`: "w",
		"./dot.jpg":       "d",
		"/root.jpg":       "r",
		"keep.jpg":        "k",
	})

	tmpPath, err := RemoveEntriesFromZip(zipPath, []string{"sub/windows.jpg", "dot.jpg", "root.jpg"})
	if err != nil {
		t.Fatalf("RemoveEntriesFromZip: %v", err)
	}
	defer os.Remove(tmpPath)

	got := readZipEntries(t, tmpPath)
	if len(got) != 1 || got["keep.jpg"] != "k" {
		t.Errorf("expected only keep.jpg, got %v", got)
	}
}

func TestRemoveEntriesFromZip_KeepsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permissions")
	}
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{"a.jpg": "a", "b.jpg": "b"})
	if err := os.Chmod(zipPath, 0o644); err != nil {
		t.Fatal(err)
	}

	tmpPath, err := RemoveEntriesFromZip(zipPath, []string{"a.jpg"})
	if err != nil {
		t.Fatalf("RemoveEntriesFromZip: %v", err)
	}
	defer os.Remove(tmpPath)

	info, err := os.Stat(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestRemoveEntriesFromZip_AllEntries(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{
		"a.jpg": "a",
		"b.jpg": "b",
	})

	tmpPath, err := RemoveEntriesFromZip(zipPath, []string{"a.jpg", "b.jpg"})
	if err != nil {
		t.Fatalf("RemoveEntriesFromZip: %v", err)
	}
	if tmpPath != "" {
		t.Errorf("expected no replacement zip, got %q", tmpPath)
	}
}

func TestRemoveEntriesFromZip_PreservesContent(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	want := "the quick brown fox"
	makeTestZip(t, zipPath, "", map[string]string{
		"keep.jpg":   want,
		"remove.jpg": "trash",
	})

	tmpPath, err := RemoveEntriesFromZip(zipPath, []string{"remove.jpg"})
	if err != nil {
		t.Fatalf("RemoveEntriesFromZip: %v", err)
	}
	defer os.Remove(tmpPath)

	got := readZipEntries(t, tmpPath)
	if got["keep.jpg"] != want {
		t.Errorf("keep.jpg content = %q, want %q", got["keep.jpg"], want)
	}
}

func TestRemoveEntriesFromZip_PreservesComment(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	wantComment := "Created by Stash"
	makeTestZip(t, zipPath, wantComment, map[string]string{
		"keep.jpg":   "data",
		"remove.jpg": "trash",
	})

	tmpPath, err := RemoveEntriesFromZip(zipPath, []string{"remove.jpg"})
	if err != nil {
		t.Fatalf("RemoveEntriesFromZip: %v", err)
	}
	defer os.Remove(tmpPath)

	if got := readZipComment(t, tmpPath); got != wantComment {
		t.Errorf("archive comment = %q, want %q", got, wantComment)
	}
}

func TestRemoveEntriesFromZip_SubdirectoryEntry(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{
		"subdir/keep.jpg":   "keep",
		"subdir/remove.jpg": "remove",
	})

	tmpPath, err := RemoveEntriesFromZip(zipPath, []string{"subdir/remove.jpg"})
	if err != nil {
		t.Fatalf("RemoveEntriesFromZip: %v", err)
	}
	defer os.Remove(tmpPath)

	got := readZipEntries(t, tmpPath)
	if _, present := got["subdir/remove.jpg"]; present {
		t.Error("subdir/remove.jpg should not be in output zip")
	}
	if got["subdir/keep.jpg"] != "keep" {
		t.Errorf("subdir/keep.jpg content = %q, want %q", got["subdir/keep.jpg"], "keep")
	}
}

func TestRemoveEntriesFromZip_InvalidPath(t *testing.T) {
	_, err := RemoveEntriesFromZip("/nonexistent/path/gallery.zip", []string{"a.jpg"})
	if err == nil {
		t.Error("expected error for non-existent zip, got nil")
	}
}

func TestDecodeZipEntryNames_UTF8(t *testing.T) {
	files := []*zip.File{
		{FileHeader: zip.FileHeader{Name: "hello.jpg"}},
		{FileHeader: zip.FileHeader{Name: "world.jpg"}},
	}

	got := decodeZipEntryNames(files, "", "test.zip")
	want := []string{"hello.jpg", "world.jpg"}

	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDecodeZipEntryNames_Empty(t *testing.T) {
	got := decodeZipEntryNames(nil, "", "test.zip")
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestDecodeZipEntryNames_NonUTF8ShiftJIS(t *testing.T) {
	// Shift-JIS encoding of "テスト.jpg" (te-su-to = test in Japanese)
	shiftJISName := "\x83\x65\x83\x58\x83\x67.jpg"
	files := []*zip.File{
		{FileHeader: zip.FileHeader{Name: shiftJISName}},
	}

	got := decodeZipEntryNames(files, "", "test.zip")

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	// The name should have been decoded to valid UTF-8 (different from the raw bytes)
	if got[0] == shiftJISName {
		t.Error("expected name to be decoded from Shift-JIS, but it was returned unchanged")
	}
}

func TestDecodeZipEntryNames_LengthMatchesInput(t *testing.T) {
	files := []*zip.File{
		{FileHeader: zip.FileHeader{Name: "a.jpg"}},
		{FileHeader: zip.FileHeader{Name: "b.jpg"}},
		{FileHeader: zip.FileHeader{Name: "c.jpg"}},
	}

	got := decodeZipEntryNames(files, "", "test.zip")
	if len(got) != len(files) {
		t.Errorf("len = %d, want %d", len(got), len(files))
	}
}
