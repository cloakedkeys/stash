package manager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/models"
)

// UploadVideo saves one file and scans only that file. It never rolls back a
// published file: scan errors must tell the caller where the file was saved.
func (s *Manager) UploadVideo(ctx context.Context, library, name string, src io.Reader) (*models.VideoFile, error) {
	return uploadVideo(ctx, s.Config, library, name, src, s.ScanFile)
}

func uploadVideo(ctx context.Context, cfg *config.Config, library, name string, src io.Reader, scan func(context.Context, ScanFileInput) (models.File, error)) (*models.VideoFile, error) {
	path, err := saveVideoUpload(ctx, cfg, library, name, src)
	if err != nil {
		return nil, err
	}
	f, err := scan(ctx, ScanFileInput{Path: path})
	if err != nil {
		return nil, fmt.Errorf("file was saved at %s, but scanning failed: %w", path, err)
	}
	v, ok := f.(*models.VideoFile)
	if !ok || v == nil {
		return nil, fmt.Errorf("file was saved at %s, but scanning did not return a video file", path)
	}
	return v, nil
}

func videoUploadPath(cfg *config.Config, library, name string) (string, error) {
	if library == "" {
		return "", fmt.Errorf("choose a configured video library")
	}
	library = filepath.Clean(library)
	eligible := false
	for _, l := range cfg.GetStashPaths() {
		if l != nil && filepath.Clean(l.Path) == library {
			if l.ExcludeVideo {
				return "", fmt.Errorf("library %q does not allow video", library)
			}
			eligible = true
		}
	}
	if !eligible {
		return "", fmt.Errorf("destination must exactly match a configured video library; subfolders are not allowed")
	}

	// Use only a basename, and reject path-like names on every operating system.
	base := filepath.Base(name)
	if name == "" || base == "." || base == ".." || base != name || strings.ContainsAny(name, `/\<>:"|?*`) || strings.ContainsFunc(name, unicode.IsControl) || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return "", fmt.Errorf("invalid upload filename %q", name)
	}
	device := strings.ToUpper(strings.TrimRight(strings.SplitN(base, ".", 2)[0], " ."))
	reserved := device == "CON" || device == "PRN" || device == "AUX" || device == "NUL" || device == "CONIN$" || device == "CONOUT$"
	if strings.HasPrefix(device, "COM") || strings.HasPrefix(device, "LPT") {
		suffix := device[3:]
		reserved = reserved || (len([]rune(suffix)) == 1 && strings.Contains("123456789¹²³", suffix))
	}
	if reserved {
		return "", fmt.Errorf("Windows reserved upload filename %q", name)
	}
	if !fsutil.MatchExtension(base, cfg.GetVideoExtensions()) {
		return "", fmt.Errorf("upload filename %q must have a configured video extension", name)
	}
	return filepath.Join(library, base), nil
}

func saveVideoUpload(ctx context.Context, cfg *config.Config, library, name string, src io.Reader) (path string, err error) {
	return saveVideoUploadWithLink(ctx, cfg, library, name, src, os.Link)
}

func saveVideoUploadWithLink(ctx context.Context, cfg *config.Config, library, name string, src io.Reader, link func(string, string) error) (path string, err error) {
	path, err = videoUploadPath(cfg, library, name)
	if err != nil {
		return "", err
	}
	limit := cfg.GetMaxUploadSize()
	if limit <= 0 {
		return "", fmt.Errorf("max_upload_size must be positive to upload video")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".stash-upload-*")
	if err != nil {
		return "", fmt.Errorf("creating upload temporary file: %w", err)
	}
	published := false
	defer func() {
		// Close before unlinking, including on Windows. Only the temporary name
		// belongs to cleanup; never delete the destination after publication.
		_ = tmp.Close()
		if cleanupErr := os.Remove(tmp.Name()); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			if published {
				err = errors.Join(err, fmt.Errorf("file was saved at %s, but removing temporary file failed: %w", path, cleanupErr))
			} else {
				err = errors.Join(err, fmt.Errorf("removing upload temporary file: %w", cleanupErr))
			}
		}
	}()

	n, err := io.Copy(tmp, io.LimitReader(src, limit+1))
	if err != nil {
		return "", fmt.Errorf("writing upload: %w", err)
	}
	if n > limit {
		return "", fmt.Errorf("upload exceeds max_upload_size (%d bytes); raise max_upload_size in config.yml (MiB) and restart Stash", limit)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := tmp.Chmod(0o644); err != nil {
		return "", fmt.Errorf("setting upload permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("closing upload: %w", err)
	}
	// Link is atomic and never replaces an existing name, on Unix and Windows.
	// Both names are in the destination directory, on the same filesystem.
	if err := link(tmp.Name(), path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("a file named %q already exists; nothing was overwritten", name)
		}
		// Filesystems without hard links still reserve the destination exclusively
		// before renaming, so another upload cannot claim the same name.
		reservation, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				return "", fmt.Errorf("a file named %q already exists; nothing was overwritten", name)
			}
			return "", fmt.Errorf("reserving upload destination: %w", err)
		}
		closeErr := reservation.Close()
		if closeErr != nil {
			return "", errors.Join(fmt.Errorf("closing upload reservation: %w", closeErr), os.Remove(path))
		}
		if err := os.Rename(tmp.Name(), path); err != nil {
			return "", errors.Join(fmt.Errorf("renaming upload: %w", err), os.Remove(path))
		}
	}
	published = true
	return path, nil
}
