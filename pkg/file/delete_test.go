package file

import (
	"io/fs"
	"os"
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
