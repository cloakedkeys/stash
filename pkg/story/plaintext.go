package story

import (
	"html"
	"regexp"
	"strings"
)

var manyBlankLines = regexp.MustCompile(`\n{3,}`)

// attribute is one attribute with a value, such as href="x", class=y or title='t', or
// one of HTML's attributes that needs no value, such as async or nowrap. Any other bare
// word, as in <Style is everything>, means it isn't a tag.
const attribute = `\s+(?:[^\s"'=<>/]+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s"'=<>` + "`" + `]+)|` + booleanAttributes + `)`

// booleanAttributes are HTML attributes that are written without a value.
const booleanAttributes = `(?:allowfullscreen|async|autofocus|autoplay|checked|controls|crossorigin|default|defer|disabled|formnovalidate|hidden|inert|ismap|itemscope|loop|multiple|muted|nomodule|novalidate|nowrap|open|playsinline|readonly|required|reversed|scoped|selected)\b`

// markup finds what is clearly markup, and nothing else:
//   - group 1: a comment that closes, or a script, style, SVG or maths drawing whose
//     opening tag has a tag's shape and whose closing tag exists. These are dropped
//     with what's inside them.
//   - group 2: the name of a tag whose attributes are all attributes, such as <i>,
//     </p>, <a href="x"> or <td nowrap>.
//
// A tag with other bare words after its name, such as <I know what you did> or
// <Style is everything>, doesn't match, so it stays as written: showing a real tag as
// text is the better mistake in a story than losing the story's own words.
var markup = regexp.MustCompile(`(?is)(<!--.*?-->` +
	`|<script(?:` + attribute + `)*\s*>.*?</script\s*>` +
	`|<style(?:` + attribute + `)*\s*>.*?</style\s*>` +
	`|<svg(?:` + attribute + `)*\s*>.*?</svg\s*>` +
	`|<math(?:` + attribute + `)*\s*>.*?</math\s*>)` +
	`|</?([a-z][a-z0-9]*)(?:` + attribute + `)*\s*/?>`)

// htmlElements are the names of HTML's elements. Only these can be markup, so
// a story that writes <whispers> or <Message received> keeps those words.
var htmlElements = map[string]bool{}

// blockElements start a new paragraph.
var blockElements = map[string]bool{}

func init() {
	for _, name := range strings.Fields(`a abbr acronym address area article aside audio
		b base basefont bdi bdo big blink blockquote body br button canvas caption center
		cite code col colgroup data datalist dd del details dfn dialog dir div dl dt em
		embed fieldset figcaption figure font footer form frame frameset h1 h2 h3 h4 h5 h6
		head header hgroup hr html i iframe img input ins kbd label legend li link main map
		mark marquee math menu meta meter nav nobr noframes noscript object ol optgroup option
		output p param picture pre progress q rp rt ruby s samp script search section select
		slot small source span strike strong style sub summary sup svg table tbody td
		template textarea tfoot th thead time title tr track tt u ul var video wbr`) {
		htmlElements[name] = true
	}
	// HTML's elements that a browser shows on their own lines
	for _, name := range strings.Fields(`address article aside blockquote caption center
		dd details dialog dir div dl dt fieldset figcaption figure footer form h1 h2 h3 h4
		h5 h6 header hgroup hr legend li main menu nav ol p pre search section summary
		table tbody tfoot thead tr ul`) {
		blockElements[name] = true
	}
}

// cellElements are table cells: the end of one is a space, so cells don't run together.
var cellElements = map[string]bool{"td": true, "th": true}

// PlainText turns a story's text from a website into plain text, since the reader
// shows plain text. Only what is clearly markup is removed, and everything else is
// kept as written:
//   - Formatting such as italics is dropped. <br> starts a new line, and a paragraph
//     or block starts after a blank line.
//   - Scripts, styles, SVG and maths drawings, and comments are dropped with what's
//     inside them, but only when they close.
//   - Entities such as &amp; become the characters they stand for, so &lt;i&gt;
//     shows as <i> rather than being taken as markup.
//   - Zero bytes are dropped, and line endings and blank lines are tidied.
func PlainText(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	var b strings.Builder
	text := 0 // start of the text not yet written
	for _, m := range markup.FindAllStringSubmatchIndex(s, -1) {
		var lineBreak string
		if m[2] < 0 {
			name := strings.ToLower(s[m[4]:m[5]])
			if !htmlElements[name] {
				continue // not markup: it's part of the story, so it stays in the text
			}
			switch {
			case name == "br":
				lineBreak = "\n"
			case blockElements[name]:
				lineBreak = "\n\n"
			case cellElements[name] && s[m[0]+1] == '/':
				lineBreak = " "
			}
		}
		b.WriteString(html.UnescapeString(s[text:m[0]]))
		b.WriteString(lineBreak)
		text = m[1]
	}
	b.WriteString(html.UnescapeString(s[text:]))

	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t ")
	}
	out := manyBlankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return strings.TrimSpace(out)
}
