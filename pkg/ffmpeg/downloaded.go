package ffmpeg

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var ErrAlreadyInstalled = errors.New("FFmpeg is already installed.")

// HasDownloadedFFmpeg reports whether either binary is present in Stash's download
// directory. Do not use executable resolution here: PATH and configured binaries
// are not managed by the download button.
func HasDownloadedFFmpeg(configDir string) bool {
	for _, name := range []string{getFFMpegFilename(), getFFProbeFilename()} {
		if info, err := os.Lstat(filepath.Join(configDir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// RemoveDownloadedFFmpeg removes only the two files managed by the downloader.
// Missing files are harmless, including when only one binary remains.
func RemoveDownloadedFFmpeg(configDir string) (bool, error) {
	removed := false
	for _, name := range []string{getFFMpegFilename(), getFFProbeFilename()} {
		filename := filepath.Join(configDir, name)
		info, err := os.Lstat(filename)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return removed, err
		}
		if info.IsDir() {
			return removed, &os.PathError{Op: "remove", Path: filename, Err: os.ErrInvalid}
		}
		if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
			return removed, err
		} else if err == nil {
			removed = true
		}
	}
	return removed, nil
}

// IsDownloadedFile reports whether path names one of the two files the
// downloader puts in configDir. A configured path pointing at one of them must
// be cleared when they're removed, or Stash keeps looking for the missing file
// instead of finding another FFmpeg.
func IsDownloadedFile(configDir, path string) bool {
	if path == "" {
		return false
	}
	// A relative path is opened from the working directory, so compare it
	// from there too.
	path, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	if configDir, err = filepath.Abs(configDir); err != nil {
		return false
	}
	for _, name := range []string{getFFMpegFilename(), getFFProbeFilename()} {
		downloaded := filepath.Join(configDir, name)
		if path == downloaded || (runtime.GOOS == "windows" && strings.EqualFold(path, downloaded)) {
			return true
		}
	}
	return false
}
