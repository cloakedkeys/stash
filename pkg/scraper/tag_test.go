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
		{`\PL`, "3d", true},
		{`\PL`, "plain", false},
		// (?-i) turns case back on, so patterns can ask for capital letters
		{"(?-i)BAR", "BAR", true},
		{"(?-i)BAR", "bar", false},
		{`(?-i)^\p{Lu}+$`, "VR", true},
		{`(?-i)^\p{Lu}+$`, "Vr", false},
		{`(?-i)[[:upper:]]{2}`, "4K HDR", true},
		{`(?-i)[[:upper:]]{2}`, "4k hdr", false},
		// non-ASCII names still ignore case
		{"^ÉCOLE$", "école", true},
		{"^straße$", "STRASSE", false},
		// the Turkish capital dotted I and the Kelvin sign match as before
		{"istanbul", "İstanbul", true},
		{"ISTANBUL", "İSTANBUL", true},
		{`^\w+$`, "İzmir", true},
		{`\W`, "İzmir", false},
		{`^k\b`, "K", true},
		// every branch of an alternation ignores case
		{"^FOO$|^Bar$", "foo", true},
		{"^FOO$|^Bar$", "BAR", true},
		{"^FOO$|^Bar$", "baz", false},
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
