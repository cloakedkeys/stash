package story

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlainText(t *testing.T) {
	for _, c := range []struct {
		name, in, want string
	}{
		{
			// what Literotica sends: paragraphs split by blank lines, with light markup
			"paragraphs with markup",
			"<i>Author's Note: hello.</i>\n\nShe said <strong>no</strong>, and <u>meant</u> it.\n\nThe end.",
			"Author's Note: hello.\n\nShe said no, and meant it.\n\nThe end.",
		},
		{
			"line breaks and paragraph tags",
			"<p>One line<br>next line<br/>last line</p><p>Second paragraph</p>",
			"One line\nnext line\nlast line\n\nSecond paragraph",
		},
		{
			"entities become characters",
			"Fish &amp; chips &lt;3 &quot;quoted&quot; caf&eacute; &#8212; done",
			"Fish & chips <3 \"quoted\" café — done",
		},
		{
			"markup in the text never survives as markup",
			"Before<script>alert('x')</script><style>p{}</style> after <img src=x onerror=alert(1)>!",
			"Before after !",
		},
		{
			"text that's already plain is kept, including a less-than sign",
			"a < b and c > d\nnext line",
			"a < b and c > d\nnext line",
		},
		{
			"line endings and blank lines are tidied",
			"\r\n\r\nFirst  \r\n\r\n\r\n\r\nSecond\rThird\n\n",
			"First\n\nSecond\nThird",
		},
		{"empty", "", ""},
		{"only markup", "<p> </p><br>", ""},
	} {
		assert.Equal(t, c.want, PlainText(c.in), c.name)
	}
}
