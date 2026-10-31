package file

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stashapp/stash/pkg/models"
)

// fakeZipStore holds zip file records by ID and records updates.
type fakeZipStore struct {
	files   map[models.FileID]*models.BaseFile
	updates int
}

// newFakeZipStore stores a record for each path, with IDs 1, 2, ...
func newFakeZipStore(paths ...string) *fakeZipStore {
	s := &fakeZipStore{files: make(map[models.FileID]*models.BaseFile)}
	for i, p := range paths {
		id := models.FileID(i + 1)
		s.files[id] = &models.BaseFile{ID: id, Path: p, Size: 1}
	}
	return s
}

func (s *fakeZipStore) Find(ctx context.Context, ids ...models.FileID) ([]models.File, error) {
	var ret []models.File
	for _, id := range ids {
		if f, ok := s.files[id]; ok {
			ret = append(ret, f)
		}
	}
	return ret, nil
}

func (s *fakeZipStore) Update(ctx context.Context, f models.File) error {
	s.updates++
	s.files[f.Base().ID] = f.Base()
	return nil
}

func zipNames(t *testing.T, path string) []string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %s: %v", path, err)
	}
	defer r.Close()
	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return names
}

func sameNames(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: got %v, want %v", what, got, want)
		}
	}
}

// files in dir other than the named ones, such as left-over rewrites
func strayFiles(t *testing.T, dir string, known ...string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	isKnown := make(map[string]bool)
	for _, k := range known {
		isKnown[k] = true
	}
	var ret []string
	for _, e := range entries {
		if !isKnown[e.Name()] {
			ret = append(ret, e.Name())
		}
	}
	return ret
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func TestDeleter_ZipRewrite_CommitRemovesEntriesAndStoresSize(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{"keep.jpg": "keep", "remove.jpg": "remove"})
	store := newFakeZipStore(zipPath)

	d, _ := newDeleterWithFake(t)
	d.ZipEntry(1, zipPath, "remove.jpg")
	if err := d.PrepareZipRewrites(context.Background(), store); err != nil {
		t.Fatal(err)
	}

	// the new zip is in place before the transaction commits, with the original set aside
	sameNames(t, "before commit", zipNames(t, zipPath), "keep.jpg")
	sameNames(t, "set-aside original", zipNames(t, zipPath+deleteFileSuffix), "keep.jpg", "remove.jpg")
	if got, want := store.files[1].Size, fileSize(t, zipPath); got != want {
		t.Errorf("stored size = %d, want the rewritten zip's %d", got, want)
	}

	d.Commit()

	sameNames(t, "after commit", zipNames(t, zipPath), "keep.jpg")
	if stray := strayFiles(t, dir, "gallery.zip"); len(stray) != 0 {
		t.Errorf("left behind: %v", stray)
	}
	if d.zips != nil {
		t.Error("Commit should clear the zip tracking")
	}
}

func TestDeleter_ZipRewrite_OncePerZip(t *testing.T) {
	dir := t.TempDir()
	zip1 := filepath.Join(dir, "one.zip")
	zip2 := filepath.Join(dir, "two.zip")
	makeTestZip(t, zip1, "", map[string]string{"a.jpg": "a", "b.jpg": "b", "c.jpg": "c"})
	makeTestZip(t, zip2, "", map[string]string{"d.jpg": "d", "e.jpg": "e"})
	store := newFakeZipStore(zip1, zip2)

	d, _ := newDeleterWithFake(t)
	d.ZipEntry(1, zip1, "a.jpg")
	d.ZipEntry(2, zip2, "d.jpg")
	d.ZipEntry(1, zip1, "b.jpg")
	if err := d.PrepareZipRewrites(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	// nothing is pending, so a second call does nothing
	if err := d.PrepareZipRewrites(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if store.updates != 2 {
		t.Errorf("updates = %d, want 2", store.updates)
	}
	d.Commit()

	sameNames(t, "one.zip", zipNames(t, zip1), "c.jpg")
	sameNames(t, "two.zip", zipNames(t, zip2), "e.jpg")
	if stray := strayFiles(t, dir, "one.zip", "two.zip"); len(stray) != 0 {
		t.Errorf("left behind: %v", stray)
	}
}

// entries marked after a first PrepareZipRewrites are removed by the next one
func TestDeleter_ZipRewrite_EntriesAfterPrepare(t *testing.T) {
	for _, commit := range []bool{true, false} {
		dir := t.TempDir()
		zipPath := filepath.Join(dir, "gallery.zip")
		makeTestZip(t, zipPath, "", map[string]string{"a.jpg": "a", "b.jpg": "b", "c.jpg": "c"})
		store := newFakeZipStore(zipPath)

		d, _ := newDeleterWithFake(t)
		d.ZipEntry(1, zipPath, "a.jpg")
		if err := d.PrepareZipRewrites(context.Background(), store); err != nil {
			t.Fatal(err)
		}
		d.ZipEntry(1, zipPath, "b.jpg")
		if err := d.PrepareZipRewrites(context.Background(), store); err != nil {
			t.Fatal(err)
		}
		if got, want := store.files[1].Size, fileSize(t, zipPath); got != want {
			t.Errorf("stored size = %d, want %d", got, want)
		}

		if commit {
			d.Commit()
			sameNames(t, "after commit", zipNames(t, zipPath), "c.jpg")
		} else {
			// rollback brings back the zip as it was before the first rewrite
			d.Rollback()
			sameNames(t, "after rollback", zipNames(t, zipPath), "a.jpg", "b.jpg", "c.jpg")
		}
		if stray := strayFiles(t, dir, "gallery.zip"); len(stray) != 0 {
			t.Errorf("commit=%v left behind: %v", commit, stray)
		}
	}
}

func TestDeleter_ZipRewrite_RollbackRestoresZip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{"keep.jpg": "keep", "remove.jpg": "remove"})
	before := fileSize(t, zipPath)

	d, _ := newDeleterWithFake(t)
	d.ZipEntry(1, zipPath, "remove.jpg")
	if err := d.PrepareZipRewrites(context.Background(), newFakeZipStore(zipPath)); err != nil {
		t.Fatal(err)
	}
	d.Rollback()

	sameNames(t, "after rollback", zipNames(t, zipPath), "keep.jpg", "remove.jpg")
	if after := fileSize(t, zipPath); after != before {
		t.Errorf("size after rollback = %d, want the original %d", after, before)
	}
	if stray := strayFiles(t, dir, "gallery.zip"); len(stray) != 0 {
		t.Errorf("left behind after rollback: %v", stray)
	}
}

// a zip that can't be rewritten fails the transaction, and the zip is untouched
func TestDeleter_ZipRewrite_PrepareFailsTheTransaction(t *testing.T) {
	cases := []struct {
		name  string
		entry string
		store func(zipPath string) *fakeZipStore
	}{
		{"zip not in the database", "remove.jpg", func(string) *fakeZipStore { return newFakeZipStore() }},
	}
	for _, c := range cases {
		dir := t.TempDir()
		zipPath := filepath.Join(dir, "gallery.zip")
		makeTestZip(t, zipPath, "", map[string]string{"keep.jpg": "keep", "remove.jpg": "remove"})

		d, _ := newDeleterWithFake(t)
		d.ZipEntry(1, zipPath, c.entry)
		if err := d.PrepareZipRewrites(context.Background(), c.store(zipPath)); err == nil {
			t.Fatalf("%s: expected an error", c.name)
		}
		d.Rollback()
		sameNames(t, c.name, zipNames(t, zipPath), "keep.jpg", "remove.jpg")
		if stray := strayFiles(t, dir, "gallery.zip"); len(stray) != 0 {
			t.Errorf("%s: left behind: %v", c.name, stray)
		}
	}

	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.zip")
	d, _ := newDeleterWithFake(t)
	d.ZipEntry(1, missing, "a.jpg")
	if err := d.PrepareZipRewrites(context.Background(), newFakeZipStore(missing)); err == nil {
		t.Fatal("expected an error for a zip that can't be opened")
	}
	d.Rollback()
}

// an entry that is already gone leaves the zip as it is
func TestDeleter_ZipRewrite_EntryAlreadyGone(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{"keep.jpg": "keep"})
	store := newFakeZipStore(zipPath)

	d, _ := newDeleterWithFake(t)
	d.ZipEntry(1, zipPath, "ghost.jpg")
	if err := d.PrepareZipRewrites(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	d.Commit()

	sameNames(t, "zip", zipNames(t, zipPath), "keep.jpg")
	if store.updates != 0 {
		t.Errorf("updates = %d, want 0", store.updates)
	}
	if stray := strayFiles(t, dir, "gallery.zip"); len(stray) != 0 {
		t.Errorf("left behind: %v", stray)
	}
}

// a zip that is deleted whole in the same transaction isn't rewritten
func TestDeleter_ZipRewrite_SkipsZipDeletedWhole(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{"a.jpg": "a", "b.jpg": "b"})

	d, _ := newDeleterWithFake(t)
	if err := d.Files([]string{zipPath}); err != nil {
		t.Fatal(err)
	}
	d.ZipEntry(1, zipPath, "a.jpg")
	if err := d.PrepareZipRewrites(context.Background(), newFakeZipStore(zipPath)); err != nil {
		t.Fatal(err)
	}
	d.Commit()

	if _, err := os.Stat(zipPath); !os.IsNotExist(err) {
		t.Errorf("zip should be deleted, stat err = %v", err)
	}
	if stray := strayFiles(t, dir); len(stray) != 0 {
		t.Errorf("left behind: %v", stray)
	}
}

// failRestore fails to bring back set-aside files, like a trash on a drive that went away
type failRestore struct{ fakeRenamerRemover }

func (f *failRestore) Rename(oldpath, newpath string) error {
	if filepath.Ext(oldpath) == deleteFileSuffix {
		return errors.New("restore failed")
	}
	return f.fakeRenamerRemover.Rename(oldpath, newpath)
}

// if the original can't come back, rollback keeps the rewritten zip rather than none
func TestDeleter_ZipRewrite_RollbackKeepsRewriteIfRestoreFails(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{"keep.jpg": "keep", "remove.jpg": "remove"})

	d := NewDeleter()
	d.RenamerRemover = &failRestore{}
	d.ZipEntry(1, zipPath, "remove.jpg")
	if err := d.PrepareZipRewrites(context.Background(), newFakeZipStore(zipPath)); err != nil {
		t.Fatal(err)
	}
	d.Rollback()

	sameNames(t, "library zip", zipNames(t, zipPath), "keep.jpg")
	sameNames(t, "set-aside original", zipNames(t, zipPath+deleteFileSuffix), "keep.jpg", "remove.jpg")
}

// without PrepareZipRewrites the zip is left alone, as before this change
func TestDeleter_ZipRewrite_CommitWithoutPrepareLeavesZip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "gallery.zip")
	makeTestZip(t, zipPath, "", map[string]string{"keep.jpg": "keep", "remove.jpg": "remove"})

	d, fake := newDeleterWithFake(t)
	d.ZipEntry(1, zipPath, "remove.jpg")
	d.Commit()

	sameNames(t, "unprepared", zipNames(t, zipPath), "keep.jpg", "remove.jpg")
	if len(fake.renames) != 0 {
		t.Errorf("renames = %v, want none", fake.renames)
	}
}

func TestDeleter_ZipRewrite_Trash(t *testing.T) {
	for _, commit := range []bool{true, false} {
		dir := t.TempDir()
		trash := filepath.Join(t.TempDir(), "trash")
		zipPath := filepath.Join(dir, "gallery.zip")
		makeTestZip(t, zipPath, "", map[string]string{"keep.jpg": "keep", "remove.jpg": "remove"})

		d, _ := newDeleterWithFake(t)
		d.TrashPath = trash
		d.ZipEntry(1, zipPath, "remove.jpg")
		if err := d.PrepareZipRewrites(context.Background(), newFakeZipStore(zipPath)); err != nil {
			t.Fatal(err)
		}
		trashed := filepath.Join(trash, "gallery.zip")
		// the original, with the removed image, is in the trash
		sameNames(t, "trashed original", zipNames(t, trashed), "keep.jpg", "remove.jpg")

		if commit {
			d.Commit()
			sameNames(t, "library zip after commit", zipNames(t, zipPath), "keep.jpg")
			sameNames(t, "trash after commit", zipNames(t, trashed), "keep.jpg", "remove.jpg")
		} else {
			d.Rollback()
			sameNames(t, "library zip after rollback", zipNames(t, zipPath), "keep.jpg", "remove.jpg")
			if _, err := os.Stat(trashed); !os.IsNotExist(err) {
				t.Errorf("trash should be empty after rollback, stat err = %v", err)
			}
		}
		if stray := strayFiles(t, dir, "gallery.zip"); len(stray) != 0 {
			t.Errorf("commit=%v left behind: %v", commit, stray)
		}
	}
}
