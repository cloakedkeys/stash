//go:build integration

package database_test

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/story"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pathCaseFingerprints struct{}

func (pathCaseFingerprints) CalculateFingerprints(f *models.BaseFile, opener file.Opener, useExisting bool) ([]models.Fingerprint, error) {
	r, err := opener.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	h := md5.New()
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}
	return []models.Fingerprint{{Type: models.FingerprintTypeMD5, Fingerprint: fmt.Sprintf("%x", h.Sum(nil))}}, nil
}

// Use the real scanner and filesystem, with no media tools required for text files.
func scanPathCaseLibrary(t *testing.T, ctx context.Context, root string) {
	t.Helper()
	filesystem := &file.OsFS{}
	scanner := &file.Scanner{
		FS:                    filesystem,
		Repository:            file.NewRepository(db.Repository()),
		RootPaths:             []string{root},
		FingerprintCalculator: pathCaseFingerprints{},
		FileHandlers:          []file.Handler{&story.ScanHandler{CreatorUpdater: db.Story()}},
	}
	require.NoError(t, file.SymWalk(filesystem, root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		scanned := file.ScannedFile{
			BaseFile: &models.BaseFile{
				Path:     path,
				Basename: filepath.Base(path),
				Size:     info.Size(),
				DirEntry: models.DirEntry{ModTime: file.ModTime(info)},
			},
			FS:   filesystem,
			Info: info,
		}
		if entry.IsDir() {
			_, err = scanner.ScanFolder(ctx, scanned)
		} else {
			_, err = scanner.ScanFile(ctx, scanned)
		}
		return err
	}))
}

// The scanner opens its own transactions, so these scans commit. Everything they
// create is removed again at the end.
func cleanUpPathCaseLibrary(t *testing.T, ctx context.Context, root string, galleryID int) {
	t.Helper()
	require.NoError(t, withTxn(func(ctx context.Context) error {
		if galleryID != 0 {
			if err := db.Gallery().Destroy(ctx, galleryID); err != nil {
				return err
			}
		}
		files, err := db.File().FindAllInPaths(ctx, []string{root}, true, -1, 0)
		if err != nil {
			return err
		}
		for _, f := range files {
			stories, err := db.Story().FindByFileID(ctx, f.Base().ID)
			if err != nil {
				return err
			}
			for _, s := range stories {
				if err := db.Story().Destroy(ctx, s.ID); err != nil {
					return err
				}
			}
			if err := db.File().Destroy(ctx, f.Base().ID); err != nil {
				return err
			}
		}
		folders, err := db.Folder().FindAllInPaths(ctx, []string{root}, true, -1, 0)
		if err != nil {
			return err
		}
		// children first
		sort.Slice(folders, func(i, j int) bool { return len(folders[i].Path) > len(folders[j].Path) })
		for _, f := range folders {
			if err := db.Folder().Destroy(ctx, f.ID); err != nil {
				return err
			}
		}
		return nil
	}))
}

func TestScannerUnicodeCaseRename(t *testing.T) {
	for _, kind := range []string{"folder", "file", "changed file", "library path"} {
		t.Run(kind, func(t *testing.T) {
			ctx := models.WithUserID(context.Background(), testUserID)
			root := filepath.Join(t.TempDir(), "Library")
			oldFolder := filepath.Join(root, "Été")
			oldPath := filepath.Join(oldFolder, "Écrit.txt")
			require.NoError(t, os.MkdirAll(oldFolder, 0755))
			require.NoError(t, os.WriteFile(oldPath, []byte("Before."), 0600))
			caseSensitive, err := (&file.OsFS{}).IsPathCaseSensitive(oldPath)
			require.NoError(t, err)
			if caseSensitive {
				t.Skip("requires a case-insensitive filesystem; exact lookup coverage runs on every OS")
			}
			scanRoot := root
			if kind == "library path" {
				scanRoot = oldFolder
			}
			scanPathCaseLibrary(t, ctx, scanRoot)
			var old models.File
			var folder *models.Folder
			var originalStory int
			gallery := models.NewGallery()
			require.NoError(t, withTxn(func(ctx context.Context) error {
				old, err = db.File().FindByPath(ctx, oldPath, true)
				require.NoError(t, err)
				require.NotNil(t, old)
				folder, err = db.Folder().FindByPath(ctx, oldFolder, true)
				require.NoError(t, err)
				require.NotNil(t, folder)
				gallery.Title = "Keep this gallery"
				gallery.FolderID = &folder.ID
				require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: &gallery}))
				stories, err := db.Story().FindByFileID(ctx, old.Base().ID)
				require.NoError(t, err)
				require.Len(t, stories, 1)
				originalStory = stories[0].ID
				_, err = db.Story().UpdatePartial(ctx, originalStory, models.StoryPartial{Title: models.NewOptionalString("Keep this title")})
				return err
			}))
			t.Cleanup(func() { cleanUpPathCaseLibrary(t, ctx, root, gallery.ID) })

			newRoot, newFolder, newPath := root, oldFolder, oldPath
			switch kind {
			case "folder":
				newFolder = filepath.Join(root, "été")
				require.NoError(t, os.Rename(oldFolder, newFolder))
				newPath = filepath.Join(newFolder, filepath.Base(oldPath))
			case "file", "changed file":
				newPath = filepath.Join(oldFolder, "écrit.txt")
				require.NoError(t, os.Rename(oldPath, newPath))
				if kind == "changed file" {
					// A new fingerprint prevents the older rename fallback from hiding the lookup bug.
					require.NoError(t, os.WriteFile(newPath, []byte("Changed content."), 0600))
				}
			case "library path":
				// Change only the spelling of the configured library, without renaming on disk.
				newRoot = filepath.Join(root, "été")
				newFolder = newRoot
				newPath = filepath.Join(newFolder, filepath.Base(oldPath))
			}
			scanPathCaseLibrary(t, ctx, newRoot)
			require.NoError(t, withTxn(func(ctx context.Context) error {
				currentFolder, err := db.Folder().FindByPath(ctx, newFolder, true)
				require.NoError(t, err)
				require.NotNil(t, currentFolder)
				assert.Equal(t, folder.ID, currentFolder.ID, "the gallery must keep the scanned folder")
				current, err := db.File().FindByPath(ctx, newPath, true)
				require.NoError(t, err)
				require.NotNil(t, current)
				assert.Equal(t, old.Base().ID, current.Base().ID)
				stories, err := db.Story().FindByFileID(ctx, current.Base().ID)
				require.NoError(t, err)
				require.Len(t, stories, 1)
				assert.Equal(t, originalStory, stories[0].ID)
				assert.Equal(t, "Keep this title", stories[0].Title)
				files, err := db.File().FindAllByPath(ctx, filepath.Join(newFolder, "*"), true)
				require.NoError(t, err)
				assert.Len(t, files, 1, "no duplicate file rows")
				return nil
			}))
		})
	}
}

func TestPathLookupUnicodeCase(t *testing.T) {
	for _, names := range [][2]string{{"Été", "été"}, {"ДОМ", "дом"}, {"ΩΡΑ", "ωρα"}, {"Ⱥ", "ⱥ"}} {
		runWithRollbackTxn(t, names[0], func(t *testing.T, ctx context.Context) {
			root := t.TempDir()
			folder := &models.Folder{Path: filepath.Join(root, names[0])}
			require.NoError(t, db.Folder().Create(ctx, folder))
			f := &models.BaseFile{ParentFolderID: folder.ID, Basename: names[0] + "_%.txt"}
			require.NoError(t, db.File().Create(ctx, f))
			otherFolderPath := filepath.Join(root, names[1])
			otherPath := filepath.Join(otherFolderPath, names[1]+"_%.txt")
			gotFolder, err := db.Folder().FindByPath(ctx, otherFolderPath, true)
			require.NoError(t, err)
			assert.Nil(t, gotFolder)
			gotFile, err := db.File().FindByPath(ctx, otherPath, true)
			require.NoError(t, err)
			assert.Nil(t, gotFile)
			gotFolder, err = db.Folder().FindByPath(ctx, otherFolderPath, false)
			require.NoError(t, err)
			require.NotNil(t, gotFolder)
			assert.Equal(t, folder.ID, gotFolder.ID)
			gotFile, err = db.File().FindByPath(ctx, otherPath, false)
			require.NoError(t, err)
			require.NotNil(t, gotFile)
			assert.Equal(t, f.ID, gotFile.Base().ID)
			scene := models.NewScene()
			require.NoError(t, db.Scene().Create(ctx, &scene, []models.FileID{f.ID}))
			scenes, err := db.Scene().FindByPath(ctx, otherPath)
			require.NoError(t, err)
			require.Len(t, scenes, 1)
			assert.Equal(t, scene.ID, scenes[0].ID)
			audio := models.NewAudio()
			require.NoError(t, db.Audio().Create(ctx, &audio, []models.FileID{f.ID}))
			audios, err := db.Audio().FindByPath(ctx, otherPath)
			require.NoError(t, err)
			require.Len(t, audios, 1)
			assert.Equal(t, audio.ID, audios[0].ID)
			files, err := db.File().FindAllByPath(ctx, filepath.Join(otherFolderPath, names[1]+"*"), false)
			require.NoError(t, err)
			require.Len(t, files, 1)
			assert.Equal(t, f.ID, files[0].Base().ID)
			scenes, err = db.Scene().FindByPath(ctx, filepath.Join(otherFolderPath, names[1]+"*"))
			require.NoError(t, err)
			require.Len(t, scenes, 1)
			assert.Equal(t, scene.ID, scenes[0].ID)
		})
	}
}

func TestScannerUnicodeCaseDistinctFiles(t *testing.T) {
	ctx := models.WithUserID(context.Background(), testUserID)
	root := t.TempDir()
	upper := filepath.Join(root, "Été", "Écrit.txt")
	lower := filepath.Join(root, "été", "écrit.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(upper), 0755))
	require.NoError(t, os.WriteFile(upper, []byte("Upper."), 0600))
	caseSensitive, err := (&file.OsFS{}).IsPathCaseSensitive(upper)
	require.NoError(t, err)
	if !caseSensitive {
		t.Skip("requires a case-sensitive filesystem")
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(lower), 0755))
	require.NoError(t, os.WriteFile(lower, []byte("Lower."), 0600))
	scanPathCaseLibrary(t, ctx, root)
	t.Cleanup(func() { cleanUpPathCaseLibrary(t, ctx, root, 0) })
	require.NoError(t, withTxn(func(ctx context.Context) error {
		first, err := db.File().FindByPath(ctx, upper, true)
		require.NoError(t, err)
		require.NotNil(t, first)
		second, err := db.File().FindByPath(ctx, lower, true)
		require.NoError(t, err)
		require.NotNil(t, second)
		assert.NotEqual(t, first.Base().ID, second.Base().ID)
		assert.NotEqual(t, first.Base().ParentFolderID, second.Base().ParentFolderID)
		return nil
	}))
}
