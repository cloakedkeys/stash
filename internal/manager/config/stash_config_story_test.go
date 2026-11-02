package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestKeepIncludeStory(t *testing.T) {
	yes, no := true, false
	current := StashConfigs{
		{Path: "/stories", IncludeStory: true},
		{Path: "/videos"},
	}
	inputs := []*StashConfigInput{
		{Path: "/stories"}, // left out: keeps on
		{Path: "/videos"},  // left out: keeps off
		{Path: "/new"},     // a new library: off
		{Path: "/stories-off", IncludeStory: &no},
		{Path: "/new-on", IncludeStory: &yes},
	}
	KeepIncludeStory(inputs, current)

	want := []bool{true, false, false, false, true}
	for i, in := range inputs {
		if in.IncludeStory == nil {
			t.Fatalf("%s: still left out", in.Path)
		}
		if *in.IncludeStory != want[i] {
			t.Errorf("%s: includeStory = %v, want %v", in.Path, *in.IncludeStory, want[i])
		}
	}

	// switching it off explicitly still works for an existing library
	off := []*StashConfigInput{{Path: "/stories", IncludeStory: &no}}
	KeepIncludeStory(off, current)
	if *off[0].IncludeStory {
		t.Error("an explicit false was overridden")
	}
}

func TestKeepIncludeStorySameFolder(t *testing.T) {
	current := StashConfigs{{Path: "/library/stories", IncludeStory: true}}
	tests := []struct {
		path string
		want bool
	}{
		{"/library/stories/", true},
		{"/library/./stories", true},
		{"/library/other/../stories", true},
		{"/library/stories/sub", false}, // a folder inside it is a different library
		{"/library", false},             // and so is the folder around it
	}
	if runtime.GOOS == "windows" {
		tests = append(tests, struct {
			path string
			want bool
		}{"/Library/Stories", true})
	}
	for _, test := range tests {
		in := []*StashConfigInput{{Path: test.path}}
		KeepIncludeStory(in, current)
		if *in[0].IncludeStory != test.want {
			t.Errorf("%s: includeStory = %v, want %v", test.path, *in[0].IncludeStory, test.want)
		}
	}
}

func TestKeepIncludeStoryExactPathFirst(t *testing.T) {
	// the same folder listed twice: the exact spelling's setting is kept
	current := StashConfigs{
		{Path: "/library/stories/", IncludeStory: false},
		{Path: "/library/stories", IncludeStory: true},
	}
	in := []*StashConfigInput{{Path: "/library/stories"}}
	KeepIncludeStory(in, current)
	if !*in[0].IncludeStory {
		t.Error("the library with the exact path wasn't used")
	}
}

func TestKeepIncludeStoryRealFolders(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Stories")
	other := filepath.Join(t.TempDir(), "Other")
	for _, d := range []string{dir, other} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	current := StashConfigs{{Path: dir, IncludeStory: true}}

	in := []*StashConfigInput{{Path: other}}
	KeepIncludeStory(in, current)
	if *in[0].IncludeStory {
		t.Error("a different folder was taken as the library")
	}

	// on a disk that ignores capital letters, as on Windows and usually on a Mac,
	// other capitals are the same folder
	upper := filepath.Join(filepath.Dir(dir), "STORIES")
	if _, err := os.Stat(upper); err != nil {
		t.Skip("this disk cares about capital letters")
	}
	in = []*StashConfigInput{{Path: upper}}
	KeepIncludeStory(in, current)
	if !*in[0].IncludeStory {
		t.Errorf("%s: stories turned off for the same folder", upper)
	}
}
