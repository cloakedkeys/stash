package config

import "testing"

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
