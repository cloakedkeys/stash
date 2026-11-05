package scraper

import (
	"testing"

	"github.com/stashapp/stash/pkg/models"
)

func TestFilterTagsExclusionPatterns(t *testing.T) {
	tests := []struct {
		pattern string
		tag     string
		exclude bool
	}{
		// plain patterns still ignore case, as before
		{"^solo$", "Solo", true},
		{"^SOLO$", "solo", true},
		{"^Solo$", "SOLO", true},
		// upper-case escapes keep their meaning instead of becoming their lower-case opposite
		{`^\S+$`, "OneWord", true},
		{`^\S+$`, "two words", false},
		{`^\D+$`, "abc", true},
		{`^\D+$`, "1080p", false},
		{`^\W`, "#hashtag", true},
		{`^\W`, "plain", false},
		{`\Bcam\B`, "webcams", true},
		{`\Bcam\B`, "cam", false},
	}

	for _, tt := range tests {
		res := CompileExclusionRegexps([]string{tt.pattern})
		if len(res) != 1 {
			t.Fatalf("pattern %q did not compile", tt.pattern)
		}
		kept, ignored := FilterTags(res, []*models.ScrapedTag{{Name: tt.tag}})
		got := len(ignored) == 1 && len(kept) == 0
		if got != tt.exclude {
			t.Errorf("pattern %q, tag %q: excluded = %v, want %v", tt.pattern, tt.tag, got, tt.exclude)
		}
	}
}
