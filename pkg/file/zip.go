package file

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/xWTF/chardet"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
)

var (
	ErrNotReaderAt  = errors.New("invalid reader: does not implement io.ReaderAt")
	errZipFSOpenZip = errors.New("cannot open zip file inside zip file")
)

// ZipFS is a file system backed by a zip file.
type zipFS struct {
	*zip.Reader
	zipFileCloser io.Closer
	zipPath       string
}

func newZipFS(fs models.FS, path string, size int64) (*zipFS, error) {
	reader, err := fs.Open(path)
	if err != nil {
		return nil, err
	}

	asReaderAt, _ := reader.(io.ReaderAt)
	if asReaderAt == nil {
		reader.Close()
		return nil, ErrNotReaderAt
	}

	zipReader, err := zip.NewReader(asReaderAt, size)
	if err != nil {
		reader.Close()
		return nil, err
	}

	// Detect and apply non-UTF-8 encoding for filenames.
	for i, name := range decodeZipEntryNames(zipReader.File, zipReader.Comment, path) {
		zipReader.File[i].Name = name
	}

	return &zipFS{
		Reader:        zipReader,
		zipFileCloser: reader,
		zipPath:       path,
	}, nil
}

func (f *zipFS) rel(name string) (string, error) {
	if f.zipPath == name {
		return ".", nil
	}

	relName, err := filepath.Rel(f.zipPath, name)
	if err != nil {
		// if the path is not relative to the zip path, then it's not found in the zip file,
		// so treat this as a file not found
		return "", fs.ErrNotExist
	}

	// convert relName to use slash, since zip files do so regardless
	// of os
	relName = filepath.ToSlash(relName)

	return relName, nil
}

func (f *zipFS) Stat(name string) (fs.FileInfo, error) {
	reader, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return reader.Stat()
}

func (f *zipFS) Lstat(name string) (fs.FileInfo, error) {
	return f.Stat(name)
}

func (f *zipFS) OpenZip(name string, size int64) (models.ZipFS, error) {
	return nil, errZipFSOpenZip
}

func (f *zipFS) IsPathCaseSensitive(path string) (bool, error) {
	return true, nil
}

type zipReadDirFile struct {
	fs.File
}

func (f *zipReadDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	asReadDirFile, _ := f.File.(fs.ReadDirFile)
	if asReadDirFile == nil {
		return nil, fmt.Errorf("internal error: not a ReadDirFile")
	}

	return asReadDirFile.ReadDir(n)
}

func (f *zipFS) Open(name string) (fs.ReadDirFile, error) {
	relName, err := f.rel(name)
	if err != nil {
		return nil, err
	}

	r, err := f.Reader.Open(relName)
	if err != nil {
		return nil, err
	}

	return &zipReadDirFile{
		File: r,
	}, nil
}

func (f *zipFS) Close() error {
	return f.zipFileCloser.Close()
}

// openOnly returns a ReadCloser where calling Close will close the zip fs as well.
func (f *zipFS) OpenOnly(name string) (io.ReadCloser, error) {
	r, err := f.Open(name)
	if err != nil {
		return nil, err
	}

	return &wrappedReadCloser{
		ReadCloser: r,
		outer:      f,
	}, nil
}

type wrappedReadCloser struct {
	io.ReadCloser
	outer io.Closer
}

func (f *wrappedReadCloser) Close() error {
	_ = f.ReadCloser.Close()
	return f.outer.Close()
}

// decodeZipEntryNames returns the decoded UTF-8 name for each entry in files.
// If the zip uses a non-UTF-8 encoding, chardet is used to detect and decode.
// Names are returned unchanged when detection fails or the charset is already UTF-8.
func decodeZipEntryNames(files []*zip.File, archiveComment string, zipPath string) []string {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name
	}

	var buf bytes.Buffer
	for _, f := range files {
		buf.WriteString(f.Name)
		buf.WriteString(f.Comment)
	}
	buf.WriteString(archiveComment)

	d, err := chardet.NewTextDetector().DetectBest(buf.Bytes())
	if err != nil {
		// if we can't detect the encoding, just assume it's UTF-8
		logger.Warnf("Unable to detect decoding for %s: %v", zipPath, err)
		return names
	}
	if d == nil || d.Charset == "UTF-8" {
		return names
	}

	logger.Debugf("Detected non-utf8 zip charset %s (%s): %s", d.Charset, d.Language, zipPath)
	e, _ := charset.Lookup(d.Charset)
	if e == nil {
		// if we can't find the encoding, just assume it's UTF-8
		logger.Warnf("Failed to lookup charset %s, language %s", d.Charset, d.Language)
		return names
	}

	decoder := e.NewDecoder()
	for i, f := range files {
		decoded, _, err := transform.String(decoder, f.Name)
		if err != nil {
			logger.Warnf("Failed to decode %v: %v", []byte(f.Name), err)
			continue
		}
		names[i] = decoded
	}
	return names
}

// errNothingToRemove means none of the entries are in the zip, so it can stay as it is.
var errNothingToRemove = errors.New("no entries to remove")

// zipEntryPath gives an entry's path the way zip.Reader.Open, and so the scanner,
// sees it: forward slashes, cleaned, with no leading slash or "./".
func zipEntryPath(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Clean(name)
	name = strings.TrimLeft(name, "/")
	for strings.HasPrefix(name, "../") {
		name = name[len("../"):]
	}
	return name
}

// RemoveEntriesFromZip rewrites the zip at zipPath to a new temporary file in the same
// directory, excluding all entries whose relative paths (using forward slashes) appear in
// entriesToRemove. Returns an empty path if no files remain, otherwise the path of
// the temporary file. The caller is responsible for
// atomically replacing the original zip with the temp file, or cleaning it up on rollback.
func RemoveEntriesFromZip(zipPath string, entriesToRemove []string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("opening zip %q: %w", zipPath, err)
	}
	defer r.Close()

	exclude := make(map[string]bool, len(entriesToRemove))
	for _, e := range entriesToRemove {
		exclude[e] = true
	}

	// match entries the way the scanner names them
	decodedNames := decodeZipEntryNames(r.File, r.Comment, zipPath)
	remaining := 0
	removed := make(map[string]bool, len(exclude))
	for i, name := range decodedNames {
		if n := zipEntryPath(name); exclude[n] {
			removed[n] = true
		} else if !r.File[i].FileInfo().IsDir() {
			remaining++
		}
	}
	for e := range exclude {
		if !removed[e] {
			// already gone, for example after the zip was edited outside stash
			logger.Warnf("%q is not in zip %q, so there is nothing to remove", e, zipPath)
		}
	}
	if len(removed) == 0 {
		return "", errNothingToRemove
	}

	if remaining == 0 {
		return "", nil
	}

	origInfo, err := os.Stat(zipPath)
	if err != nil {
		return "", fmt.Errorf("reading zip %q: %w", zipPath, err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(zipPath), "*.tmp")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// CreateTemp makes the file readable by its owner only; keep the zip's own permissions
	if err := os.Chmod(tmpPath, origInfo.Mode().Perm()); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("setting permissions on %q: %w", tmpPath, err)
	}

	w := zip.NewWriter(tmpFile)
	for i, f := range r.File {
		if exclude[zipEntryPath(decodedNames[i])] {
			continue
		}
		if err := w.Copy(f); err != nil {
			_ = w.Close()
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("copying zip entry %q: %w", f.Name, err)
		}
	}
	if err := w.SetComment(r.Comment); err != nil {
		_ = w.Close()
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("setting zip comment: %w", err)
	}
	if err := w.Close(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("finalizing zip: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("closing temp file: %w", err)
	}

	return tmpPath, nil
}
