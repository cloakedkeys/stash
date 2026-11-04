package image

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/require"
)

// #1139: deleting an image deletes its generated files in both layouts.
func TestMarkGeneratedFilesBothLayouts(t *testing.T) {
	const (
		checksum = "0123456789abcdef0123456789abcdef"
		other    = "0123fedcba9876543210fedcba987654"
	)
	p := paths.NewPaths(t.TempDir(), "")

	files := func(c string) []string {
		var ret []string
		ret = append(ret, p.Generated.Thumbnail(c, models.DefaultGthumbWidth).Both()...)
		ret = append(ret, p.Generated.ClipPreview(c, models.DefaultGthumbWidth).Both()...)
		return ret
	}
	for _, f := range append(files(checksum), files(other)...) {
		require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o755))
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
	}

	d := FileDeleter{Deleter: file.NewDeleter(), Paths: &p}
	require.NoError(t, d.MarkGeneratedFiles(&models.Image{ID: 1, Checksum: checksum}))
	d.Commit()

	for _, f := range files(checksum) {
		require.NoFileExists(t, f)
	}
	for _, f := range files(other) {
		require.FileExists(t, f, "another image's file")
	}
}
