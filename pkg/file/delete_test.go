package file

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// fakeRenamerRemover records Rename/Remove calls and delegates to real OS operations.
type fakeRenamerRemover struct {
	renames []renameOp
	removes []string
}

type renameOp struct{ from, to string }

func (f *fakeRenamerRemover) Rename(oldpath, newpath string) error {
	f.renames = append(f.renames, renameOp{oldpath, newpath})
	return os.Rename(oldpath, newpath)
}

func (f *fakeRenamerRemover) Remove(name string) error {
	f.removes = append(f.removes, name)
	return os.Remove(name)
}

func (f *fakeRenamerRemover) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (f *fakeRenamerRemover) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func newDeleterWithFake(t *testing.T) (*Deleter, *fakeRenamerRemover) {
	t.Helper()
	fake := &fakeRenamerRemover{}
	d := NewDeleter()
	d.RenamerRemover = fake
	return d, fake
}

func TestDeleter_ZipEntry_Accumulates(t *testing.T) {
	d, _ := newDeleterWithFake(t)

	d.ZipEntry(1, "/path/to/gallery.zip", "a.jpg")
	d.ZipEntry(1, "/path/to/gallery.zip", "b.jpg")

	entries := d.zips["/path/to/gallery.zip"].pending
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d: %v", len(entries), entries)
	}
}

func TestDeleter_ZipEntry_MultipleZips(t *testing.T) {
	d, _ := newDeleterWithFake(t)

	d.ZipEntry(1, "/zip/one.zip", "img1.jpg")
	d.ZipEntry(1, "/zip/two.zip", "img2.jpg")
	d.ZipEntry(1, "/zip/one.zip", "img3.jpg")

	if len(d.zips["/zip/one.zip"].pending) != 2 {
		t.Errorf("one.zip: expected 2 entries, got %d", len(d.zips["/zip/one.zip"].pending))
	}
	if len(d.zips["/zip/two.zip"].pending) != 1 {
		t.Errorf("two.zip: expected 1 entry, got %d", len(d.zips["/zip/two.zip"].pending))
	}
}

func TestDeleter_Rollback_ClearsZipEntries(t *testing.T) {
	d, _ := newDeleterWithFake(t)

	d.ZipEntry(1, "/path/to/gallery.zip", "a.jpg")
	d.Rollback()

	if d.zips != nil {
		t.Errorf("expected zips to be nil after Rollback, got %v", d.zips)
	}
}

func TestDeleter_NameTooLong(t *testing.T) {
	testDeleterNameTooLong(t, syscall.ENAMETOOLONG)
}

func TestDeleter_NameTooLongRealFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), strings.Repeat("a", 252))
	if err := os.WriteFile(p, []byte("original"), 0600); err != nil {
		t.Skipf("cannot create long filename: %v", err)
	}
	d := NewDeleter()
	err := d.Files([]string{p})
	if err == nil {
		t.Skip("filesystem allows the filename with .delete appended")
	}
	want := fmt.Sprintf("name of %q is too long for Stash to delete; shorten its name or remove it manually", p)
	if !strings.Contains(err.Error(), want) {
		t.Errorf("expected actionable error containing %q, got %v", want, err)
	}
	if len(d.files) != 0 {
		t.Errorf("failed rename marked files for deletion: %v", d.files)
	}
	if contents, err := os.ReadFile(p); err != nil || string(contents) != "original" {
		t.Errorf("original file changed: contents=%q, err=%v", contents, err)
	}
}

func testDeleterNameTooLong(t *testing.T, cause error) {
	t.Helper()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"errno", cause},
		{"rename", &os.LinkError{Op: "rename", Old: "video.mp4", New: "video.mp4.delete", Err: cause}},
		{"copy fallback", fmt.Errorf("copy failed: %w", &os.PathError{Op: "open", Path: "video.mp4.delete", Err: cause})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "video.mp4")
			if err := os.WriteFile(p, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			d := NewDeleter()
			rr := newRenamerRemoverImpl()
			rr.RenameFn = func(oldpath, newpath string) error {
				if oldpath != p || newpath != p+".delete" {
					t.Fatalf("unexpected rename: %q -> %q", oldpath, newpath)
				}
				return tc.err
			}
			d.RenamerRemover = rr

			err := d.Files([]string{p})
			want := fmt.Sprintf("name of %q is too long for Stash to delete; shorten its name or remove it manually", p)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("expected actionable error containing %q, got %v", want, err)
			}
			if !errors.Is(err, cause) {
				t.Errorf("error does not wrap original cause: %v", err)
			}
			if len(d.files) != 0 {
				t.Errorf("failed rename marked files for deletion: %v", d.files)
			}
			if contents, err := os.ReadFile(p); err != nil || string(contents) != "original" {
				t.Errorf("original file changed: contents=%q, err=%v", contents, err)
			}
		})
	}
}

func TestDeleter_RenameForDeleteOtherError(t *testing.T) {
	d := NewDeleter()
	rr := newRenamerRemoverImpl()
	want := &os.LinkError{Op: "rename", Old: "video.mp4", New: "video.mp4.delete", Err: fs.ErrPermission}
	rr.RenameFn = func(_, _ string) error { return want }
	d.RenamerRemover = rr
	if err := d.renameForDelete("video.mp4", false); err != want {
		t.Errorf("expected unchanged permission error, got %v", err)
	}
}
