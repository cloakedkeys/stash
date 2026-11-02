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
		{
			// a story's own use of angle brackets, for thoughts or messages, is kept
			"words in angle brackets that aren't HTML",
			"<whispers>quietly</whispers> and <Message received> <i>then</i> <3",
			"<whispers>quietly</whispers> and <Message received> then <3",
		},
		{
			// starts with a tag name, but has bare words after it, so it's the story's
			"words in angle brackets that start with a tag name",
			"<I know what you did> she thought.\n<a quiet voice> called.\n<Is anyone there?>\na<b and c>d",
			"<I know what you did> she thought.\n<a quiet voice> called.\n<Is anyone there?>\na<b and c>d",
		},
		{
			// tags whose attributes all have values are markup
			"tags with attributes",
			`<a href="x">link</a> <p class=y>para</p><span title='t' data-x = "1">span</span><br/>end`,
			"link\n\npara\n\nspan\nend",
		},
		{
			// the better mistake: a real tag with a bare attribute shows as text, though
			// its closing tag is still markup
			"a bare attribute",
			"<td nowrap>cell</td>",
			"<td nowrap>cell",
		},
		{
			// like any tag with a bare attribute, it isn't clearly markup, so it shows as
			// text; the reader shows plain text, so it can't run
			"a script with a bare attribute",
			"Before<script async>alert(1)</script><style scoped>p{}</style> after",
			"Before<script async>alert(1)<style scoped>p{} after",
		},
		{
			"drawings are dropped with what's inside them",
			"Before<svg><rect width='1'/><text>x</text></svg> after<math><mi>y</mi></math>.",
			"Before after.",
		},
		{
			// words that start with the name of an element dropped with its contents
			"words in angle brackets that start with script, style, svg or math",
			"Page one starts here. <Style is everything> she said, and carried on.\n" +
				"<Script of the play> <Math is hard> <Svg file attached> The rest.",
			"Page one starts here. <Style is everything> she said, and carried on.\n" +
				"<Script of the play> <Math is hard> <Svg file attached> The rest.",
		},
		{
			// only a script or style that closes is dropped with what's inside it
			"a style or script that never closes",
			"Before<style>p{color:red} after. <script>more",
			"Beforep{color:red} after. more",
		},
		{
			"a less-than sign before a closing tag's slash",
			"I love you </3 she said. Later x > y. After.",
			"I love you </3 she said. Later x > y. After.",
		},
		{
			"angle brackets with a question or exclamation mark",
			"<?What is that?> and <!Stop right there!>",
			"<?What is that?> and <!Stop right there!>",
		},
		{"an angle bracket that never closes", "The end <she said", "The end <she said"},
		{
			// the one case that can't be told apart from a real tag
			"words with the shape of a real tag",
			`<I said="hello"> she said`,
			"she said",
		},
		{
			// elements whose contents a browser reads as text don't change what follows
			"markup after words in angle brackets",
			"<Title of the book> she said.<br>The rest, <i>really</i>.\n" +
				"<Textarea of notes><br><Plaintext here> <i>a</i> <Iframe x> <i>b</i> <Noscript ok> <i>c</i>",
			"<Title of the book> she said.\nThe rest, really.\n" +
				"<Textarea of notes>\n<Plaintext here> a <Iframe x> b <Noscript ok> c",
		},
		{
			"comments are dropped only when they close",
			"a<!-- a note -->b <!-- c",
			"ab <!-- c",
		},
		{
			// an entity is the character it stands for, never markup
			"escaped markup stays as text",
			"&lt;i&gt;kept&lt;/i&gt; &lt;script&gt;x&lt;/script&gt;",
			"<i>kept</i> <script>x</script>",
		},
		{
			"tag names in capitals, and tags across lines",
			"<P>one</P><SCRIPT\n  type=\"x\">gone</SCRIPT><BR\n/>two",
			"one\n\ntwo",
		},
		{"zero bytes are dropped", "before\x00 after\x00", "before after"},
		{"empty", "", ""},
		{"only markup", "<p> </p><br>", ""},
	} {
		assert.Equal(t, c.want, PlainText(c.in), c.name)
	}
}
