package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/fsutil"
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
	if isReservedWindowsName(name) {
		name += "_"
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
// " (3)" and so on when the name is taken. It never replaces a file.
func writeStoryText(dir, title, text string) (string, error) {
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

// SaveStoryText saves a scraped story's text as a text file in the folder for
// downloaded stories and adds the file to the library. It returns nil when that
// folder isn't set, so the text is stored in the database instead. The scan makes a
// story for the file, which the caller joins to its own story with story.AssignFile.
func (s *Manager) SaveStoryText(ctx context.Context, title, text string) (models.File, error) {
	dir := s.Config.GetStoryDownloadPath()
	if dir == "" {
		return nil, nil
	}
	if err := CheckStoryDownloadPath(s.Config, dir); err != nil {
		return nil, err
	}

	path, err := writeStoryText(dir, title, text)
	if err != nil {
		return nil, err
	}

	f, err := s.ScanFile(ctx, ScanFileInput{Path: path})
	if err != nil {
		return nil, fmt.Errorf("the story's text was saved at %s, but adding it to the library failed: %w", path, err)
	}
	if f == nil {
		return nil, fmt.Errorf("the story's text was saved at %s, but it wasn't added to the library", path)
	}
	return f, nil
}
