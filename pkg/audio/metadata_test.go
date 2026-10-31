package audio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTagDate(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"2019", "2019"},
		{"2019-03", "2019-03"},
		{"2019-03-02", "2019-03-02"},
		{"2019-03-02T10:20:30Z", "2019-03-02"},
		{"20190302", "2019-03-02"},
		{"2019/03/02", "2019-03-02"},
		{"", ""},
		{"unknown", ""},
		{"2019-13-40", ""},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got := parseTagDate(tc.in)
			if tc.want == "" {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tc.want, got.String())
		})
	}
}

func TestParseTagTrack(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want int
	}{{"3", 3}, {"3/12", 3}, {" 07 ", 7}} {
		got := parseTagTrack(tt.in)
		require.NotNil(t, got, tt.in)
		assert.Equal(t, tt.want, *got)
	}
	for _, in := range []string{"", "A1", "-1"} {
		assert.Nil(t, parseTagTrack(in), in)
	}
}

func TestSplitTagValues(t *testing.T) {
	assert.Equal(t, []string{"A", "B", "C, D"}, splitTagValues("A; B/C, D"))
	assert.Empty(t, splitTagValues(" ; "))
}

func TestMetadataFromTags(t *testing.T) {
	got := metadataFromTags(map[string]string{
		"title":        " Song ",
		"album_artist": "Fallback",
		"artist":       "Singer",
		"description":  "About",
		"genre":        "Pop;Rock",
		"album":        "Album",
		"year":         "2001",
		"tracknumber":  "4/9",
	})

	assert.Equal(t, "Song", got.Title)
	assert.Equal(t, "About", got.Details)
	assert.Equal(t, []string{"Singer"}, got.Artists)
	assert.Equal(t, []string{"Pop", "Rock"}, got.Genres)
	assert.Equal(t, "Album", got.Album)
	require.NotNil(t, got.Date)
	assert.Equal(t, "2001", got.Date.String())
	require.NotNil(t, got.Track)
	assert.Equal(t, 4, *got.Track)
}
