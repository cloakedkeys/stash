package scene

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/require"
)

func TestDeleteGeneratedEmbeddedCaptionsIsIsolated(t *testing.T) {
	p := paths.NewPaths(t.TempDir(), "")
	own := p.Scene.GetEmbeddedSubtitleDir(12)
	neighbor := p.Scene.GetEmbeddedSubtitleDir(123)
	unrelated := filepath.Join(p.Generated.Vtt, "keep.vtt")
	for _, name := range []string{filepath.Join(own, "version.json"), filepath.Join(own, "version-2.vtt"), filepath.Join(neighbor, "keep.vtt"), unrelated} {
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0755))
		require.NoError(t, os.WriteFile(name, []byte("keep"), 0600))
	}
	// No scene hash: subtitle cleanup still runs. The usual delete transaction
	// can roll back, or commit without moving generated files to trash.
	d := FileDeleter{Deleter: file.NewDeleterWithTrash(filepath.Join(t.TempDir(), "trash")), Paths: &p}
	s := &models.Scene{ID: 12}
	require.NoError(t, d.MarkGeneratedFiles(s))
	require.NoDirExists(t, own)
	d.Rollback()
	require.FileExists(t, filepath.Join(own, "version-2.vtt"))
	require.NoError(t, d.MarkGeneratedFiles(s))
	d.Commit()
	require.NoDirExists(t, own)
	require.NoDirExists(t, own+".delete")
	require.FileExists(t, filepath.Join(neighbor, "keep.vtt"))
	require.FileExists(t, unrelated)
}
