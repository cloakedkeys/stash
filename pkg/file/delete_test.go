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

	"github.com/stashapp/stash/pkg/logger"
)

func TestDeleter_DeleteCollision(t *testing.T) {
	for _, directory := range []bool{false, true} {
		for _, commit := range []bool{false, true} {
			t.Run(fmt.Sprintf("directory=%v/commit=%v", directory, commit), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "video.mp4")
				original := path
				if directory {
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
					original = filepath.Join(path, "contents")
				}
				writeDeleteTestFile(t, original, "original")
				writeDeleteTestFile(t, path+deleteFileSuffix, "unrelated")
				d := NewDeleter()
				mark := d.Files
				if directory {
					mark = d.Dirs
				}
				if err := mark([]string{path}); !errors.Is(err, fs.ErrExist) {
					t.Errorf("expected destination collision, got %v", err)
				}
				if commit {
					d.Commit()
				} else {
					d.Rollback()
				}
				assertDeleteTestFile(t, original, "original")
				assertDeleteTestFile(t, path+deleteFileSuffix, "unrelated")
			})
		}
	}
}

func TestDeleter_RestoreCollision(t *testing.T) {
	for _, trash := range []bool{false, true} {
		t.Run(fmt.Sprintf("trash=%v", trash), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "video.mp4")
			writeDeleteTestFile(t, path, "original")
			d := NewDeleter()
			if trash {
				d.TrashPath = t.TempDir()
			}
			if err := d.Files([]string{path}); err != nil {
				t.Fatal(err)
			}
			aside := d.setAsidePath(path)
			writeDeleteTestFile(t, path, "new file")
			logs := captureDeleteWarnings(t)
			d.Rollback()
			assertDeleteTestFile(t, path, "new file")
			assertDeleteTestFile(t, aside, "original")
			assertDeleteWarningPath(t, logs, aside)
		})
	}
}

func TestDeleter_TrashPerPath(t *testing.T) {
	for _, directory := range []bool{false, true} {
		for _, bypassTrash := range []bool{false, true} {
			for _, commit := range []bool{false, true} {
				t.Run(fmt.Sprintf("directory=%v/bypassTrash=%v/commit=%v", directory, bypassTrash, commit), func(t *testing.T) {
					path := filepath.Join(t.TempDir(), "generated")
					original := path
					if directory {
						if err := os.Mkdir(path, 0700); err != nil {
							t.Fatal(err)
						}
						original = filepath.Join(path, "contents")
					}
					writeDeleteTestFile(t, original, "original")
					d := NewDeleterWithTrash(t.TempDir())
					mark := d.Files
					if directory {
						mark = d.Dirs
						if bypassTrash {
							mark = d.DirsWithoutTrash
						}
					} else if bypassTrash {
						mark = d.FilesWithoutTrash
					}
					if err := mark([]string{path}); err != nil {
						t.Fatal(err)
					}
					aside := d.setAsidePath(path)
					if bypassTrash != (aside == path+deleteFileSuffix) {
						t.Fatalf("unexpected set-aside path %q", aside)
					}
					if commit {
						d.Commit()
						if !bypassTrash {
							contents := aside
							if directory {
								contents = filepath.Join(aside, "contents")
							}
							assertDeleteTestFile(t, contents, "original")
						} else if _, err := os.Stat(aside); !errors.Is(err, fs.ErrNotExist) {
							t.Errorf("set-aside path still exists after commit: %v", err)
						}
						if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
							t.Errorf("original path still exists after commit: %v", err)
						}
					} else {
						d.Rollback()
						assertDeleteTestFile(t, original, "original")
						if _, err := os.Stat(aside); !errors.Is(err, fs.ErrNotExist) {
							t.Errorf("set-aside path still exists after rollback: %v", err)
						}
					}
				})
			}
		}
	}
}

type deleteWarningLogger struct {
	logger.LoggerImpl
	warnings []string
}

func (l *deleteWarningLogger) Warnf(format string, args ...interface{}) {
	l.warnings = append(l.warnings, fmt.Sprintf(format, args...))
}

func captureDeleteWarnings(t *testing.T) *deleteWarningLogger {
	t.Helper()
	previous := logger.Logger
	l := &deleteWarningLogger{LoggerImpl: previous}
	logger.Logger = l
	t.Cleanup(func() { logger.Logger = previous })
	return l
}

func assertDeleteWarningPath(t *testing.T, logs *deleteWarningLogger, path string) {
	t.Helper()
	for _, warning := range logs.warnings {
		if strings.Contains(warning, fmt.Sprintf("%q", path)) {
			return
		}
	}
	t.Errorf("no warning names original location %q: %v", path, logs.warnings)
}

type destinationStatError struct{ fakeRenamerRemover }

func (f *destinationStatError) Stat(path string) (fs.FileInfo, error) {
	return nil, fs.ErrPermission
}

func TestDeleter_DestinationStatError(t *testing.T) {
	fake := &destinationStatError{}
	d := NewDeleter()
	d.RenamerRemover = fake
	if err := d.renameForDelete("video.mp4", false); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("expected permission error, got %v", err)
	}
	if len(fake.renames) != 0 {
		t.Errorf("destination could not be checked, but rename was attempted: %v", fake.renames)
	}
}

func writeDeleteTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func assertDeleteTestFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Errorf("reading %q: got %q, error %v; want %q", path, got, err, want)
	}
}

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
	if err := d.renameForDelete("video.mp4", false); !errors.Is(err, want) {
		t.Errorf("expected unchanged permission error, got %v", err)
	}
}
