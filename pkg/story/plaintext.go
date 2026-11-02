package story

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var manyBlankLines = regexp.MustCompile(`\n{3,}`)

// htmlElements are the names of HTML's elements. Only these are taken as markup, so
// a story that writes <whispers> or <Message received> keeps those words.
var htmlElements = map[string]bool{}

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
}

// PlainText turns a story's text from a website into plain text, since the reader
// shows plain text. Formatting such as italics is dropped, entities such as &amp;
// become the characters they stand for, and line breaks and paragraphs are kept:
// <br> starts a new line, and a paragraph or block starts after a blank line.
// Scripts, styles, and SVG and maths drawings are dropped entirely. Anything in angle
// brackets that isn't an HTML element is kept as written. Zero bytes are dropped. Text
// that's already plain stays as it is, apart from tidied line endings and blank lines.
func PlainText(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(s))
	skip := 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}

		switch tt {
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
			}
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			// copied first, since reading the name lowercases it in place
			raw := append([]byte(nil), z.Raw()...)
			name, _ := z.TagName()
			if !htmlElements[string(name)] {
				// not markup: it's part of the story, so it's kept as written
				if skip == 0 {
					b.Write(raw)
				}
				continue
			}
			switch string(name) {
			case "script", "style", "svg", "math":
				if tt == html.StartTagToken {
					skip++
				} else if tt == html.EndTagToken && skip > 0 {
					skip--
				}
			case "br":
				b.WriteString("\n")
			case "p", "div", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6", "li", "hr", "tr", "pre":
				b.WriteString("\n\n")
			}
		}
	}

	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t ")
	}
	out := manyBlankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return strings.TrimSpace(out)
}
