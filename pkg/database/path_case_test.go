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

type pathCaseRows struct {
	folder   *models.Folder
	file     models.File
	stories  []*models.Story
	siblings []models.File
}

// readPathCase reads what the scans left behind. Nothing here may stop the
// test: a failed check inside the transaction would leave it open, and the
// cleanup would then wait on it forever.
func readPathCase(t *testing.T, folderPath, filePath string) pathCaseRows {
	t.Helper()
	var r pathCaseRows
	require.NoError(t, withTxn(func(ctx context.Context) error {
		var err error
		if r.folder, err = db.Folder().FindByPath(ctx, folderPath, true); err != nil {
			return err
		}
		if r.file, err = db.File().FindByPath(ctx, filePath, true); err != nil {
			return err
		}
		if r.file != nil {
			if r.stories, err = db.Story().FindByFileID(ctx, r.file.Base().ID); err != nil {
				return err
			}
		}
		r.siblings, err = db.File().FindAllByPath(ctx, filepath.Join(folderPath, "*"), true)
		return err
	}))
	return r
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
			before := readPathCase(t, oldFolder, oldPath)
			require.NotNil(t, before.folder)
			require.NotNil(t, before.file)
			require.Len(t, before.stories, 1)
			gallery := models.NewGallery()
			gallery.Title = "Keep this gallery"
			gallery.FolderID = &before.folder.ID
			originalStory := before.stories[0].ID
			require.NoError(t, withTxn(func(ctx context.Context) error {
				if err := db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: &gallery}); err != nil {
					return err
				}
				_, err := db.Story().UpdatePartial(ctx, originalStory, models.StoryPartial{Title: models.NewOptionalString("Keep this title")})
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
			after := readPathCase(t, newFolder, newPath)
			require.NotNil(t, after.folder)
			assert.Equal(t, before.folder.ID, after.folder.ID, "the gallery must keep the scanned folder")
			require.NotNil(t, after.file)
			assert.Equal(t, before.file.Base().ID, after.file.Base().ID)
			require.Len(t, after.stories, 1)
			assert.Equal(t, originalStory, after.stories[0].ID)
			assert.Equal(t, "Keep this title", after.stories[0].Title)
			assert.Len(t, after.siblings, 1, "no duplicate file rows")
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

// Go lower-cases both İ and I to i, but Windows keeps ai.txt and aİ.txt apart.
func TestPathLookupDistinctLetters(t *testing.T) {
	runWithRollbackTxn(t, "İ", func(t *testing.T, ctx context.Context) {
		root := t.TempDir()
		folder := &models.Folder{Path: filepath.Join(root, "ai")}
		require.NoError(t, db.Folder().Create(ctx, folder))
		f := &models.BaseFile{ParentFolderID: folder.ID, Basename: "ai.txt"}
		require.NoError(t, db.File().Create(ctx, f))
		gotFolder, err := db.Folder().FindByPath(ctx, filepath.Join(root, "aİ"), false)
		require.NoError(t, err)
		assert.Nil(t, gotFolder)
		gotFile, err := db.File().FindByPath(ctx, filepath.Join(folder.Path, "aİ.txt"), false)
		require.NoError(t, err)
		assert.Nil(t, gotFile)
		files, err := db.File().FindAllByPath(ctx, filepath.Join(folder.Path, "aİ*"), false)
		require.NoError(t, err)
		assert.Empty(t, files)
	})
}

func TestScannerDistinctLetters(t *testing.T) {
	ctx := models.WithUserID(context.Background(), testUserID)
	root := t.TempDir()
	first := filepath.Join(root, "ai.txt")
	second := filepath.Join(root, "aİ.txt")
	require.NoError(t, os.WriteFile(first, []byte("First."), 0600))
	scanPathCaseLibrary(t, ctx, root)
	t.Cleanup(func() { cleanUpPathCaseLibrary(t, ctx, root, 0) })
	require.NoError(t, os.WriteFile(second, []byte("Second."), 0600))
	if b, err := os.ReadFile(first); err != nil || string(b) != "First." {
		t.Skip("the filesystem treats ai.txt and aİ.txt as one file")
	}
	scanPathCaseLibrary(t, ctx, root)
	a := readPathCase(t, root, first).file
	b := readPathCase(t, root, second).file
	require.NotNil(t, a, "ai.txt keeps its row")
	require.NotNil(t, b, "aİ.txt gets its own row")
	assert.NotEqual(t, a.Base().ID, b.Base().ID)
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
	first := readPathCase(t, filepath.Dir(upper), upper).file
	second := readPathCase(t, filepath.Dir(lower), lower).file
	require.NotNil(t, first)
	require.NotNil(t, second)
	assert.NotEqual(t, first.Base().ID, second.Base().ID)
	assert.NotEqual(t, first.Base().ParentFolderID, second.Base().ParentFolderID)
}
