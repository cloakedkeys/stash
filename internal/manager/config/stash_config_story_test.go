package config

import (
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
