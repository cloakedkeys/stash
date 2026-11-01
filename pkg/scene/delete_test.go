package scene

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkGeneratedFilesMarkerFolder(t *testing.T) {
	for _, withTrash := range []bool{false, true} {
		name := "without trash"
		if withTrash {
			name = "with trash"
		}
		t.Run(name, func(t *testing.T) {
			for _, present := range []bool{true, false} {
				name := "missing folder"
				if present {
					name = "existing folder"
				}
				t.Run(name, func(t *testing.T) {
					root := t.TempDir()
					p := paths.NewPaths(root, "")
					s := &models.Scene{ID: 12, Checksum: "0123456789abcdef0123456789abcdef"}
					folder := filepath.Join(p.Generated.Markers, s.Checksum)
					preview := filepath.Join(folder, "nested", "10.mp4")
					if present {
						require.NoError(t, os.MkdirAll(filepath.Dir(preview), 0755))
						require.NoError(t, os.WriteFile(preview, []byte("preview"), 0600))
					}

					trash := filepath.Join(root, "trash")
					require.NoError(t, os.Mkdir(trash, 0755))
					deleter := file.NewDeleter()
					if withTrash {
						deleter = file.NewDeleterWithTrash(trash)
					}
					d := FileDeleter{Deleter: deleter, Paths: &p, FileNamingAlgo: models.HashAlgorithmMd5}
					require.NoError(t, d.MarkGeneratedFiles(s))
					if present {
						// Marking only renames the folder; its contents survive until commit.
						asidePreview := filepath.Join(folder+".delete", "nested", "10.mp4")
						assert.FileExists(t, asidePreview)
						d.Rollback()
						require.FileExists(t, preview)
						require.NoError(t, d.MarkGeneratedFiles(s))
						assert.FileExists(t, asidePreview)
					}
					d.Commit()
					require.NoDirExists(t, folder)
					require.NoDirExists(t, folder+".delete")
					entries, err := os.ReadDir(trash)
					require.NoError(t, err)
					require.Empty(t, entries)
				})
			}
		})
	}
}
