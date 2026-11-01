package story

import (
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

func encode(t *testing.T, e encoding.Encoding, s string) []byte {
	t.Helper()
	b, err := e.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("encoding %q: %v", s, err)
	}
	return b
}

func TestDecodeText(t *testing.T) {
	const (
		french = "Il était une fois, dans un café près de la forêt, une jeune fille qui rêvait d'été et de liberté. Elle s'appelait Hélène."
		quotes = "She said “yes” — and smiled. It’s the end…"
		jaText = "昔々、ある所におじいさんとおばあさんが住んでいました。おじいさんは山へ芝刈りに、おばあさんは川へ洗濯に行きました。"
		zhText = "从前有一座山，山里有一座庙，庙里有一个老和尚在给小和尚讲故事。"
		ruText = "Жили-были дед да баба. Была у них курочка Ряба. Снесла курочка яичко, не простое — золотое."
	)

	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"utf-8", []byte(french), french},
		{"utf-8 with byte order mark", append([]byte{0xEF, 0xBB, 0xBF}, french...), french},
		{"empty", []byte{}, ""},
		{"short latin-1", encode(t, charmap.ISO8859_1, "café"), "café"},
		{"latin-1", encode(t, charmap.ISO8859_1, french), french},
		{"windows-1252 quotes", encode(t, charmap.Windows1252, quotes), quotes},
		{"shift-jis", encode(t, japanese.ShiftJIS, jaText), jaText},
		{"gbk", encode(t, simplifiedchinese.GBK, zhText), zhText},
		{"windows-1251", encode(t, charmap.Windows1251, ruText), ruText},
		{"utf-16 little endian", encode(t, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), french), french},
		{"utf-16 big endian", encode(t, unicode.UTF16(unicode.BigEndian, unicode.UseBOM), jaText), jaText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(DecodeText(tt.in)); got != tt.want {
				t.Errorf("DecodeText() = %q, want %q", got, tt.want)
			}
		})
	}
}
