package story

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/xWTF/chardet"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
)

// detectSampleSize is how much of a file is used to guess its encoding.
const detectSampleSize = 64 * 1024

var (
	utf8BOM    = []byte{0xEF, 0xBB, 0xBF}
	utf32LEBOM = []byte{0xFF, 0xFE, 0x00, 0x00}
	utf32BEBOM = []byte{0x00, 0x00, 0xFE, 0xFF}
)

// DecodeText returns the text of a story file as UTF-8. A byte order mark
// decides the encoding when there is one. Text that is already valid UTF-8
// is returned as it is. Otherwise the encoding is guessed, and Windows-1252
// is used when the guess isn't usable, since it gives every byte a character.
func DecodeText(data []byte) []byte {
	if bytes.HasPrefix(data, utf8BOM) {
		return data[len(utf8BOM):]
	}
	// before UTF-16, since the UTF-32 little-endian mark starts with the UTF-16 one
	if bytes.HasPrefix(data, utf32LEBOM) || bytes.HasPrefix(data, utf32BEBOM) {
		return decodeWith(utf32.UTF32(utf32.BigEndian, utf32.ExpectBOM), data)
	}
	if len(data) >= 2 && ((data[0] == 0xFF && data[1] == 0xFE) || (data[0] == 0xFE && data[1] == 0xFF)) {
		return decodeWith(unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM), data)
	}
	if utf8.Valid(data) {
		return data
	}
	return decodeWith(guessEncoding(data), data)
}

func guessEncoding(data []byte) encoding.Encoding {
	sample := data
	if len(sample) > detectSampleSize {
		sample = sample[:detectSampleSize]
	}

	d, err := chardet.NewTextDetector().DetectBest(sample)
	if err != nil || d == nil {
		return charmap.Windows1252
	}

	// the text isn't valid UTF-8, so a UTF-8 or ASCII guess is wrong
	name := strings.ToLower(d.Charset)
	if name == "utf-8" || name == "us-ascii" || name == "ascii" {
		return charmap.Windows1252
	}

	e, _ := charset.Lookup(d.Charset)
	if e == nil {
		return charmap.Windows1252
	}
	return e
}

func decodeWith(e encoding.Encoding, data []byte) []byte {
	out, err := e.NewDecoder().Bytes(data)
	if err != nil {
		// a decoder that can't read the text falls back to one that reads anything
		out, _ = charmap.Windows1252.NewDecoder().Bytes(data)
	}
	return out
}
