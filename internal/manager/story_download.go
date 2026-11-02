package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

// storyFileExt is the extension of a saved story's text file.
const storyFileExt = ".txt"

// maxStoryNameBytes keeps a saved story's file name, with " (n)" and the extension,
// within the 255-byte limit of common filesystems.
const maxStoryNameBytes = 200

// CheckStoryDownloadPath reports why dir can't hold saved stories: it must be in a
// library that includes stories, and .txt must be a story extension, so that a scan
// keeps what's saved there as stories.
func CheckStoryDownloadPath(cfg *config.Config, dir string) error {
	stash := cfg.GetStashPaths().GetStashFromDirPath(dir)
	if stash == nil || !stash.IncludeStory {
		return fmt.Errorf("the folder for downloaded stories, %s, must be inside a library that includes stories", dir)
	}
	if !fsutil.MatchExtension("story"+storyFileExt, cfg.GetStoryExtensions()) {
		return fmt.Errorf("the folder for downloaded stories needs txt in the story extensions")
	}
	return nil
}

// storyFileName makes a file name from a story's title that's safe on Windows,
// macOS, Linux and Android, without the extension.
func storyFileName(title string) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\<>:"|?*`, r) {
			return '_'
		}
		return r
	}, title)
	name = strings.Join(strings.Fields(name), " ")

	if len(name) > maxStoryNameBytes {
		name = name[:maxStoryNameBytes]
		for !utf8.ValidString(name) {
			name = name[:len(name)-1]
		}
	}

	// Windows drops trailing dots and spaces, and "." and ".." aren't names
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "Story"
	}
	// at the front, since the device name before any dot is what's reserved
	if isReservedWindowsName(name) {
		name = "_" + name
	}
	return name
}

// isReservedWindowsName reports whether Windows keeps a file name for a device, which
// it does whatever the extension.
func isReservedWindowsName(name string) bool {
	device := strings.ToUpper(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " ."))
	switch device {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	if strings.HasPrefix(device, "COM") || strings.HasPrefix(device, "LPT") {
		switch device[3:] {
		case "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³":
			return true
		}
	}
	return false
}

// writeStoryText writes text to a new file in dir named after the title, adding " (2)",
// " (3)" and so on when the name is taken, on disk or, if held is given, by a file in
// the library that has gone missing. It never replaces a file.
func writeStoryText(dir, title, text string, held func(path string) (bool, error)) (string, error) {
	if err := fsutil.EnsureDir(dir); err != nil {
		return "", fmt.Errorf("making the folder for downloaded stories: %w", err)
	}

	name := storyFileName(title)
	for n := 1; n <= 1000; n++ {
		base := name
		if n > 1 {
			base = fmt.Sprintf("%s (%d)", name, n)
		}
		path := filepath.Join(dir, base+storyFileExt)

		if held != nil {
			// a scan would take a new file there for the missing one, with its story
			isHeld, err := held(path)
			if err != nil {
				return "", fmt.Errorf("checking the library for %s: %w", path, err)
			}
			if isHeld {
				continue
			}
		}

		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("saving the story's text: %w", err)
		}

		_, err = f.WriteString(text)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return "", errors.Join(fmt.Errorf("saving the story's text: %w", err), os.Remove(path))
		}
		return path, nil
	}
	return "", fmt.Errorf("saving the story's text: too many files named %q", name+storyFileExt)
}

// nameHeld reports whether path's name is one of names, ignoring case as some
// filesystems do. It folds every alphabet, which the database's own comparison doesn't.
func nameHeld(names []string, path string) bool {
	base := filepath.Base(path)
	return slices.ContainsFunc(names, func(name string) bool { return strings.EqualFold(name, base) })
}

// folderTree finds folders in the library.
type folderTree interface {
	// TopFolders returns the folders without a parent, which include each library's
	// that isn't inside another.
	TopFolders(ctx context.Context) ([]*models.Folder, error)
	FindByParentFolderID(ctx context.Context, parentFolderID models.FolderID) ([]*models.Folder, error)
}

// repositoryFolders is the library's folders in the database.
type repositoryFolders struct {
	models.FolderReaderWriter
}

func (r repositoryFolders) TopFolders(ctx context.Context) ([]*models.Folder, error) {
	all := -1
	result, err := r.Query(ctx, models.FolderQueryOptions{
		QueryOptions: models.QueryOptions{FindFilter: &models.FindFilterType{PerPage: &all}},
		FolderFilter: &models.FolderFilterType{
			ParentFolder: &models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierIsNull},
		},
	})
	if err != nil {
		return nil, err
	}
	return result.Resolve(ctx)
}

// pathParts splits a path into its folder names, the first being the volume or empty.
func pathParts(path string) []string {
	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	if len(parts) > 1 && parts[len(parts)-1] == "" {
		// a volume's root, such as C:\ or /
		parts = parts[:len(parts)-1]
	}
	return parts
}

// foldersAt finds the folders in the library at dir, ignoring case in every alphabet,
// as some filesystems do and the database doesn't. It starts from the top folders that
// hold dir and walks down to it, comparing each folder's name. On a filesystem that
// minds case, folders whose names differ only in case are all returned, so that a name
// used in any of them is avoided.
func foldersAt(ctx context.Context, tree folderTree, dir string) ([]*models.Folder, error) {
	want := pathParts(dir)
	tops, err := tree.TopFolders(ctx)
	if err != nil {
		return nil, err
	}

	// the folders found so far, with how many of dir's names they cover
	type step struct {
		folder *models.Folder
		depth  int
	}
	var steps []step
	for _, f := range tops {
		have := pathParts(f.Path)
		if len(have) <= len(want) && slices.EqualFunc(have, want[:len(have)], strings.EqualFold) {
			steps = append(steps, step{f, len(have)})
		}
	}

	var found []*models.Folder
	for len(steps) > 0 {
		var next []step
		for _, s := range steps {
			if s.depth == len(want) {
				found = append(found, s.folder)
				continue
			}
			children, err := tree.FindByParentFolderID(ctx, s.folder.ID)
			if err != nil {
				return nil, err
			}
			for _, c := range children {
				if strings.EqualFold(filepath.Base(c.Path), want[s.depth]) {
					next = append(next, step{c, s.depth + 1})
				}
			}
		}
		steps = next
	}
	return found, nil
}

// storyFolderFileNames lists the names of the files in the library that are in dir,
// including ones that have gone missing.
func (s *Manager) storyFolderFileNames(ctx context.Context, dir string) (names []string, err error) {
	err = s.Repository.WithReadTxn(ctx, func(ctx context.Context) error {
		folders, err := foldersAt(ctx, repositoryFolders{s.Repository.Folder}, dir)
		if err != nil {
			return err
		}
		for _, folder := range folders {
			files, err := s.Repository.File.FindByFolderID(ctx, folder.ID)
			if err != nil {
				return err
			}
			for _, f := range files {
				names = append(names, f.Base().Basename)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing the library's files in %s: %w", dir, err)
	}
	return names, nil
}

// SaveStoryText saves a scraped story's text as a text file in the folder for
// downloaded stories and adds the file to the library. It returns nil when that
// folder isn't set, or when the file turns out to be one already in the library, so
// the text is stored in the database instead. Otherwise the file is new, and the scan
// made a story for it, which the caller joins to its own story with story.AssignFile.
func (s *Manager) SaveStoryText(ctx context.Context, title, text string) (models.File, error) {
	dir := s.Config.GetStoryDownloadPath()
	if dir == "" {
		return nil, nil
	}
	if err := CheckStoryDownloadPath(s.Config, dir); err != nil {
		return nil, err
	}

	inLibrary, err := s.storyFolderFileNames(ctx, dir)
	if err != nil {
		return nil, err
	}
	held := func(path string) (bool, error) { return nameHeld(inLibrary, path), nil }
	path, err := writeStoryText(dir, title, text, held)
	if err != nil {
		return nil, err
	}

	r, err := s.scanFile(ctx, ScanFileInput{Path: path})
	if err == nil && (r == nil || r.File == nil) {
		err = errors.New("the scan skipped it")
	}
	if err != nil {
		// so a failed save leaves nothing behind
		return nil, errors.Join(fmt.Errorf("adding %s to the library: %w", path, err), os.Remove(path))
	}
	if !r.New {
		// The scan took the file for one already in the library that had gone missing,
		// as its text is the same (a name held by one is skipped above). It stays with
		// whatever has that file, which keeps its details, and nil has the text stored
		// instead.
		logger.Infof("%s matched a missing file already in the library, so it was kept as that file and the story's text was stored", path)
		return nil, nil
	}
	return r.File, nil
}
