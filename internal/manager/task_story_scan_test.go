package manager

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
)

type fixedCounter int

func (c fixedCounter) CountByFileID(ctx context.Context, fileID models.FileID) (int, error) {
	return int(c), nil
}

// storyLibraries sets up a library that includes stories, one that doesn't, and one
// that only has stories, and returns their folders.
func storyLibraries(t *testing.T) (stories, videos, textOnly string, stashes config.StashConfigs) {
	t.Helper()
	config.InitializeEmpty()
	previous := instance
	instance = &Manager{Config: config.GetInstance()}
	t.Cleanup(func() { instance = previous })

	root := t.TempDir()
	stories = filepath.Join(root, "stories")
	videos = filepath.Join(root, "videos")
	textOnly = filepath.Join(root, "text-only")
	stashes = config.StashConfigs{
		{Path: stories, IncludeStory: true},
		{Path: videos},
		{Path: textOnly, ExcludeVideo: true, ExcludeAudio: true, ExcludeImage: true, IncludeStory: true},
	}
	instance.Config.SetInterface(config.Stash, stashes)
	if got := instance.Config.GetStashPaths(); len(got) != 3 || !got[0].IncludeStory || got[1].IncludeStory {
		t.Fatalf("libraries weren't set: %+v", got)
	}
	return stories, videos, textOnly, stashes
}

func writeTestFile(t *testing.T, path string) os.FileInfo {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("Once upon a time."), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestStoryFilesAreScannedInLibrariesThatIncludeStories(t *testing.T) {
	stories, videos, textOnly, stashes := storyLibraries(t)
	ctx := context.Background()

	filter := scanFilter{
		extensionConfig:   newExtensionConfig(instance.Config),
		stashPaths:        stashes,
		generatedPath:     filepath.Join(filepath.Dir(stories), "generated"),
		stashIgnoreFilter: file.NewStashIgnoreFilter(),
	}
	verify := verifyFilter{scanFilter: filter}

	for _, c := range []struct {
		name  string
		path  string
		story bool
		scan  bool
	}{
		{"a text file in a story library", filepath.Join(stories, "tale.txt"), true, true},
		{"markdown", filepath.Join(stories, "notes.md"), true, true},
		{"an upper-case extension", filepath.Join(stories, "LOUD.TXT"), true, true},
		{"a text file in a library without stories", filepath.Join(videos, "readme.txt"), false, false},
		{"html isn't plain text", filepath.Join(stories, "page.html"), false, false},
		{"a zip is a gallery, not a story", filepath.Join(stories, "pack.zip"), false, true},
		{"a video in a story library", filepath.Join(stories, "clip.mp4"), false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			info := writeTestFile(t, c.path)
			if got := UseAsStory(c.path); got != c.story {
				t.Errorf("UseAsStory = %v, want %v", got, c.story)
			}
			if got := storyFileFilter(ctx, &models.BaseFile{Path: c.path}); got != c.story {
				t.Errorf("story handler filter = %v, want %v", got, c.story)
			}
			if got := filter.Accept(ctx, c.path, info, ""); got != c.scan {
				t.Errorf("scan accepted = %v, want %v", got, c.scan)
			}
			// a file the scan doesn't take is marked missing, so a clean removes its story
			if got := verify.Accept(ctx, c.path, info, ""); got != c.scan {
				t.Errorf("verify accepted = %v, want %v", got, c.scan)
			}
		})
	}

	t.Run("a text file inside a zip in a story library", func(t *testing.T) {
		inner := filepath.Join(stories, "pack.zip", "inner.txt")
		if !UseAsStory(inner) {
			t.Error("UseAsStory = false, want true")
		}
	})

	t.Run("a folder in a library that only has stories", func(t *testing.T) {
		dir := filepath.Join(textOnly, "sub")
		info := writeTestFile(t, filepath.Join(dir, "tale.txt"))
		dirInfo, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		f := filter
		f.videoExcludeRegex = []*regexp.Regexp{regexp.MustCompile(".*")}
		if !f.Accept(ctx, dir, dirInfo, "") {
			t.Error("scan skipped the folder")
		}
		if !f.Accept(ctx, filepath.Join(dir, "tale.txt"), info, "") {
			t.Error("scan skipped the story")
		}
		v := verifyFilter{scanFilter: f}
		if !v.Accept(ctx, dir, dirInfo, "") {
			t.Error("verify marked the folder missing")
		}
	})
}

func TestStoryHandlerRunsForTextFilesWithoutAStory(t *testing.T) {
	stories, videos, _, _ := storyLibraries(t)
	ctx := context.Background()

	for _, c := range []struct {
		name    string
		path    string
		stories int
		want    bool
	}{
		{"a new story file", filepath.Join(stories, "tale.txt"), 0, true},
		{"a story file that already has a story", filepath.Join(stories, "tale.txt"), 1, false},
		{"a text file in a library without stories", filepath.Join(videos, "readme.txt"), 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &handlerRequiredFilter{
				extensionConfig: newExtensionConfig(instance.Config),
				StoryFinder:     fixedCounter(c.stories),
			}
			if got := f.Accept(ctx, &models.BaseFile{Path: c.path}); got != c.want {
				t.Errorf("handler required = %v, want %v", got, c.want)
			}
		})
	}
}
