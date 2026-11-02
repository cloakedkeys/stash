package story

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var manyBlankLines = regexp.MustCompile(`\n{3,}`)

// PlainText turns a story's text from a website into plain text, since the reader
// shows plain text. Formatting such as italics is dropped, entities such as &amp;
// become the characters they stand for, and line breaks and paragraphs are kept:
// <br> starts a new line, and a paragraph or block starts after a blank line.
// Scripts and styles are dropped entirely. Text that's already plain stays as it is,
// apart from tidied line endings and blank lines.
func PlainText(s string) string {
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
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style":
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
