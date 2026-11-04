package savedfilter

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	includes      = models.CriterionModifierIncludes
	excludes      = models.CriterionModifierExcludes
	equals        = models.CriterionModifierEquals
	notEquals     = models.CriterionModifierNotEquals
	matchesRegex  = models.CriterionModifierMatchesRegex
	notMatchRegex = models.CriterionModifierNotMatchesRegex
)

func TestConvertLegacyWildcardCriterion(t *testing.T) {
	tests := []struct {
		name         string
		modifier     models.CriterionModifier
		value        string
		wantModifier models.CriterionModifier
		wantValue    string
		wantOK       bool
	}{
		{"includes one word", includes, "part_1", matchesRegex, `(?is)(part.1)`, true},
		{"includes words", includes, "foo  part_1\tbar%", matchesRegex, `(?is)(foo|part.1|bar.*)`, true},
		{"includes trims", includes, "  part_1  ", matchesRegex, `(?is)(part.1)`, true},
		{"includes quoted phrase", includes, `"part_1 two"`, matchesRegex, `(?is)(part.1 two)`, true},
		{"includes half quoted is a phrase", includes, `"part_1 two`, matchesRegex, `(?is)(part.1 two)`, true},
		{"excludes words", excludes, "a_ b", notMatchRegex, `(?is)(a.|b)`, true},
		{"excludes quoted phrase", excludes, `"100% real"`, notMatchRegex, `(?is)(100.* real)`, true},
		{"equals", equals, "part_1", matchesRegex, `(?is)^part.1$`, true},
		{"equals keeps spaces", equals, " 50% off ", matchesRegex, `(?is)^ 50.* off $`, true},
		{"not equals", notEquals, "%.mp4", notMatchRegex, `(?is)^.*\.mp4$`, true},
		{"regex characters are quoted", equals, `a.b+c*(d)[e]{f}|g^h$i?_`, matchesRegex, `(?is)^a\.b\+c\*\(d\)\[e\]\{f\}\|g\^h\$i\?.$`, true},
		// \ was a plain character on both databases: SQLite LIKE has no escape
		// character unless one is given, and PostgreSQL used ESCAPE ''
		{"backslash is a plain character", equals, `C:\videos\part_1`, matchesRegex, `(?is)^C:\\videos\\part.1$`, true},
		{"escaped-looking wildcard is still a wildcard", includes, `a\_b`, matchesRegex, `(?is)(a\\.b)`, true},
		{"no wildcard", includes, "part-1", includes, "part-1", false},
		{"regex left alone", matchesRegex, "part_1", matchesRegex, "part_1", false},
		{"not regex left alone", notMatchRegex, "a%", notMatchRegex, "a%", false},
		{"is null left alone", models.CriterionModifierIsNull, "_", models.CriterionModifierIsNull, "_", false},
	}

	// these have only ASCII, which both databases read the same way
	for _, dialect := range []LegacyDialect{LegacySQLite, LegacyPostgres} {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				gotModifier, gotValue, ok := ConvertLegacyWildcardCriterion(dialect, tt.modifier, tt.value)
				assert.Equal(t, tt.wantOK, ok)
				assert.Equal(t, tt.wantModifier, gotModifier)
				assert.Equal(t, tt.wantValue, gotValue)
				if ok {
					_, err := regexp.Compile(gotValue)
					assert.NoError(t, err)
				}
			})
		}
	}
}

// SQLite's LIKE ignored case only for A to Z, PostgreSQL's ILIKE for every
// letter.
func TestConvertLegacyWildcardCriterionLetterCase(t *testing.T) {
	tests := []struct {
		name      string
		dialect   LegacyDialect
		modifier  models.CriterionModifier
		value     string
		wantValue string
	}{
		{"sqlite includes", LegacySQLite, includes, "été_a", `(?is)((?-i:é)t(?-i:é).a)`},
		{"sqlite equals", LegacySQLite, equals, "Straße_", `(?is)^Stra(?-i:ß)e.$`},
		{"sqlite letters without case", LegacySQLite, equals, "日本_", `(?is)^日本.$`},
		{"postgres includes", LegacyPostgres, includes, "été_a", `(?is)(été.a)`},
		{"postgres equals", LegacyPostgres, equals, "Straße_", `(?is)^Straße.$`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got, ok := ConvertLegacyWildcardCriterion(tt.dialect, tt.modifier, tt.value)
			require.True(t, ok)
			assert.Equal(t, tt.wantValue, got)
		})
	}

	_, sqlite, _ := ConvertLegacyWildcardCriterion(LegacySQLite, includes, "été_a")
	assert.True(t, regexp.MustCompile(sqlite).MatchString("été-A"))
	assert.False(t, regexp.MustCompile(sqlite).MatchString("ÉTÉ-A"))
	_, postgres, _ := ConvertLegacyWildcardCriterion(LegacyPostgres, includes, "été_a")
	assert.True(t, regexp.MustCompile(postgres).MatchString("ÉTÉ-A"))
}

// likeMatch is SQLite's LIKE without an escape character: % is any run of
// characters, _ is one character, and ASCII letters match either case.
func likeMatch(pattern, s string) bool {
	return likeMatchFolded(asciiOnlyLower(pattern), asciiOnlyLower(s))
}

// ilikeMatch is PostgreSQL's ILIKE with no escape character: every letter
// matches either case.
func ilikeMatch(pattern, s string) bool {
	return likeMatchFolded(strings.ToLower(pattern), strings.ToLower(s))
}

func dialectLikeMatch(dialect LegacyDialect, pattern, s string) bool {
	if dialect == LegacyPostgres {
		return ilikeMatch(pattern, s)
	}
	return likeMatch(pattern, s)
}

func likeMatchFolded(pattern, s string) bool {
	p := []rune(pattern)
	r := []rune(s)
	var match func(i, j int) bool
	match = func(i, j int) bool {
		if i == len(p) {
			return j == len(r)
		}
		switch p[i] {
		case '%':
			for k := j; k <= len(r); k++ {
				if match(i+1, k) {
					return true
				}
			}
			return false
		case '_':
			return j < len(r) && match(i+1, j+1)
		default:
			return j < len(r) && p[i] == r[j] && match(i+1, j+1)
		}
	}
	return match(0, 0)
}

func asciiOnlyLower(s string) string {
	b := []rune(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// oldMatch is what a criterion with a LIKE-based modifier matched before.
func oldMatch(dialect LegacyDialect, modifier models.CriterionModifier, value, s string) bool {
	switch modifier {
	case includes, excludes:
		found := false
		for _, t := range legacySearchTerms(value) {
			if dialectLikeMatch(dialect, "%"+t+"%", s) {
				found = true
			}
		}
		if modifier == excludes {
			return !found
		}
		return found
	case equals:
		return dialectLikeMatch(dialect, value, s)
	case notEquals:
		return !dialectLikeMatch(dialect, value, s)
	}
	panic("unexpected modifier")
}

func newMatch(modifier models.CriterionModifier, value, s string) bool {
	m := regexp.MustCompile(value).MatchString(s)
	if modifier == notMatchRegex {
		return !m
	}
	return m
}

// The rewritten criteria match exactly the text the old ones did.
func TestConvertLegacyWildcardCriterionMatchesTheSame(t *testing.T) {
	values := []string{
		"part_1", "PART_1", "part%", "%1", "_art_1", "p%1", "foo part_1", `"part 1%"`, `"100% real"`,
		"a.b_", "(x)_", `C:\v_`, "line_one%end", "%", "_", "__",
		"été_a", "ÉTÉ%", "Straße_", "日本_",
	}
	texts := []string{
		"", "part_1", "part-1", "part 1", "parts", "Part-1", "PART 1", "apart_12", "part1", "part",
		"100% real", "100 percent real", "100real", "a.b!", "axb!", "(x)y", "xx(x)y", `C:\v1`, `C:v1`,
		"line-one\nmiddle\nend", "line one end", "x", "xy", "foo",
		"été_a", "été-A", "ÉTÉ-A", "Été-a", "ÉTÉ", "étéx", "Straße!", "STRASSE!", "straẞe!", "日本語",
	}
	for _, dialect := range []LegacyDialect{LegacySQLite, LegacyPostgres} {
		for _, modifier := range []models.CriterionModifier{includes, excludes, equals, notEquals} {
			for _, value := range values {
				newModifier, newValue, ok := ConvertLegacyWildcardCriterion(dialect, modifier, value)
				require.True(t, ok, value)
				for _, s := range texts {
					assert.Equalf(t, oldMatch(dialect, modifier, value, s), newMatch(newModifier, newValue, s),
						"dialect %d %s %q on %q (regex %q)", dialect, modifier, value, s, newValue)
				}
			}
		}
	}
}

func decode(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	m, err := decodeJSONObject(s)
	require.NoError(t, err)
	return m
}

func encode(t *testing.T, m map[string]interface{}) string {
	t.Helper()
	s, err := encodeJSON(m)
	require.NoError(t, err)
	return s
}

func TestConvertLegacyWildcards(t *testing.T) {
	tests := []struct {
		name        string
		mode        models.FilterMode
		filter      string
		want        string
		wantChanged bool
		wantNotes   []string
	}{
		{
			name:        "scene title includes",
			mode:        models.FilterModeScenes,
			filter:      `{"title":{"value":"part_1","modifier":"INCLUDES"}}`,
			want:        `{"title":{"modifier":"MATCHES_REGEX","value":"(?is)(part.1)"}}`,
			wantChanged: true,
		},
		{
			name:   "no wildcard is untouched",
			mode:   models.FilterModeScenes,
			filter: `{"title":{"value":"part-1","modifier":"INCLUDES"},"details":{"value":"x","modifier":"EQUALS"}}`,
		},
		{
			name: "enum-like values are untouched",
			mode: models.FilterModeScenes,
			filter: `{"resolution":{"value":"FULL_HD","modifier":"EQUALS"},` +
				`"orientation":{"value":["LANDSCAPE"],"modifier":"INCLUDES"},` +
				`"is_missing":"stash_id"}`,
		},
		{
			name:   "performer gender and circumcised are untouched",
			mode:   models.FilterModePerformers,
			filter: `{"gender":{"value":"TRANSGENDER_MALE","modifier":"EQUALS"},"circumcised":{"value":["CUT"],"modifier":"INCLUDES"}}`,
		},
		{
			name:        "performer text fields",
			mode:        models.FilterModePerformers,
			filter:      `{"hair_color":{"value":"dark_","modifier":"NOT_EQUALS"},"name":{"value":"a%","modifier":"EXCLUDES"}}`,
			want:        `{"hair_color":{"modifier":"NOT_MATCHES_REGEX","value":"(?is)^dark.$"},"name":{"modifier":"NOT_MATCHES_REGEX","value":"(?is)(a.*)"}}`,
			wantChanged: true,
		},
		{
			name:   "legacy height and career length are untouched",
			mode:   models.FilterModePerformers,
			filter: `{"height":{"value":"1_0","modifier":"EQUALS"},"career_length":{"value":"2000_","modifier":"EQUALS"}}`,
		},
		{
			name:   "scene phash is untouched",
			mode:   models.FilterModeScenes,
			filter: `{"phash":{"value":"ab_cd","modifier":"EQUALS"}}`,
		},
		{
			name:        "list field excludes is untouched and noted, other modifiers are rewritten",
			mode:        models.FilterModePerformers,
			filter:      `{"aliases":{"value":"a_b","modifier":"EXCLUDES"},"url":{"value":"site_x","modifier":"INCLUDES"}}`,
			want:        `{"aliases":{"modifier":"EXCLUDES","value":"a_b"},"url":{"modifier":"MATCHES_REGEX","value":"(?is)(site.x)"}}`,
			wantChanged: true,
			wantNotes:   []string{`aliases: "excludes" on a field with several values can't be rewritten as a regular expression`},
		},
		{
			name:      "tag aliases excludes is untouched",
			mode:      models.FilterModeTags,
			filter:    `{"aliases":{"value":"%x","modifier":"EXCLUDES"}}`,
			wantNotes: []string{`aliases: "excludes" on a field with several values can't be rewritten as a regular expression`},
		},
		{
			name:      "captions are untouched",
			mode:      models.FilterModeScenes,
			filter:    `{"captions":{"value":"en_US","modifier":"INCLUDES"}}`,
			wantNotes: []string{"captions: captions are left as they are"},
		},
		{
			name: "nested sub-filters",
			mode: models.FilterModeScenes,
			filter: `{"AND":{"title":{"value":"a_","modifier":"EQUALS"},"OR":{"code":{"value":"b%","modifier":"INCLUDES"}}},` +
				`"performers_filter":{"name":{"value":"c_","modifier":"INCLUDES"},"aliases":{"value":"d_","modifier":"EXCLUDES"}},` +
				`"files_filter":{"path":{"value":"e_","modifier":"INCLUDES"},"video_file_filter":{"format":{"value":"m_4","modifier":"EQUALS"},"resolution":{"value":"FULL_HD","modifier":"EQUALS"},"captions":{"value":"f_","modifier":"EQUALS"}}}}`,
			want: `{"AND":{"OR":{"code":{"modifier":"MATCHES_REGEX","value":"(?is)(b.*)"}},"title":{"modifier":"MATCHES_REGEX","value":"(?is)^a.$"}},` +
				`"files_filter":{"path":{"modifier":"MATCHES_REGEX","value":"(?is)(e.)"},"video_file_filter":{"captions":{"modifier":"EQUALS","value":"f_"},"format":{"modifier":"MATCHES_REGEX","value":"(?is)^m.4$"},"resolution":{"modifier":"EQUALS","value":"FULL_HD"}}},` +
				`"performers_filter":{"aliases":{"modifier":"EXCLUDES","value":"d_"},"name":{"modifier":"MATCHES_REGEX","value":"(?is)(c.)"}}}`,
			wantChanged: true,
			wantNotes: []string{
				"files_filter.video_file_filter.captions: captions are left as they are",
				`performers_filter.aliases: "excludes" on a field with several values can't be rewritten as a regular expression`,
			},
		},
		{
			name:        "gallery path",
			mode:        models.FilterModeGalleries,
			filter:      `{"path":{"value":"%/part_1","modifier":"EQUALS"}}`,
			want:        `{"path":{"modifier":"MATCHES_REGEX","value":"(?is)^.*/part.1$"}}`,
			wantChanged: true,
		},
		{
			name:        "story title, which has no json name",
			mode:        models.FilterModeStories,
			filter:      `{"title":{"value":"ch_1","modifier":"INCLUDES"}}`,
			want:        `{"title":{"modifier":"MATCHES_REGEX","value":"(?is)(ch.1)"}}`,
			wantChanged: true,
		},
		{
			name:        "groups and the old movies mode",
			mode:        models.FilterModeMovies,
			filter:      `{"director":{"value":"x_","modifier":"EQUALS"}}`,
			want:        `{"director":{"modifier":"MATCHES_REGEX","value":"(?is)^x.$"}}`,
			wantChanged: true,
		},
		{
			name:        "scene marker scene filter",
			mode:        models.FilterModeSceneMarkers,
			filter:      `{"scene_filter":{"title":{"value":"x_","modifier":"EQUALS"}}}`,
			want:        `{"scene_filter":{"title":{"modifier":"MATCHES_REGEX","value":"(?is)^x.$"}}}`,
			wantChanged: true,
		},
		{
			name:   "unknown fields and modes are untouched",
			mode:   models.FilterModeScenes,
			filter: `{"no_such_field":{"value":"a_","modifier":"EQUALS"},"title":{"value":5,"modifier":"EQUALS"}}`,
		},
		{
			name:   "unknown mode",
			mode:   models.FilterMode("NOPE"),
			filter: `{"title":{"value":"a_","modifier":"EQUALS"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := decode(t, tt.filter)
			before := encode(t, m)
			changed, notes := ConvertLegacyWildcards(tt.mode, m)
			assert.Equal(t, tt.wantChanged, changed)
			assert.Equal(t, tt.wantNotes, notes)
			if tt.wantChanged {
				assert.Equal(t, tt.want, encode(t, m))
			} else {
				assert.Equal(t, before, encode(t, m))
			}
		})
	}
}

func TestConvertLegacyWildcardsJSON(t *testing.T) {
	t.Run("keeps other values as they were", func(t *testing.T) {
		in := `{"title":{"value":"a_<b>&","modifier":"EQUALS"},"rating100":{"value":12345678901234567890,"modifier":"EQUALS"},"tags":{"value":{"items":[{"id":"1","label":"x"}],"depth":0},"modifier":"INCLUDES"}}`
		out, changed, notes, err := ConvertLegacyWildcardsJSON(LegacySQLite, models.FilterModeScenes, in)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Empty(t, notes)
		assert.Equal(t, `{"rating100":{"modifier":"EQUALS","value":12345678901234567890},"tags":{"modifier":"INCLUDES","value":{"depth":0,"items":[{"id":"1","label":"x"}]}},"title":{"modifier":"MATCHES_REGEX","value":"(?is)^a.<b>&$"}}`, out)
	})
	t.Run("unchanged returns the input", func(t *testing.T) {
		in := `{ "title": {"value":"x","modifier":"EQUALS"} }`
		out, changed, _, err := ConvertLegacyWildcardsJSON(LegacySQLite, models.FilterModeScenes, in)
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, in, out)
	})
	t.Run("empty and null", func(t *testing.T) {
		for _, in := range []string{"", "null"} {
			out, changed, _, err := ConvertLegacyWildcardsJSON(LegacySQLite, models.FilterModeScenes, in)
			require.NoError(t, err)
			assert.False(t, changed)
			assert.Equal(t, in, out)
		}
	})
	t.Run("bad json", func(t *testing.T) {
		_, changed, _, err := ConvertLegacyWildcardsJSON(LegacySQLite, models.FilterModeScenes, "{")
		assert.Error(t, err)
		assert.False(t, changed)
	})
}

func TestLegacyWildcardSearchNote(t *testing.T) {
	assert.Equal(t, `search text "part_1": _ and % in it now match only themselves, and search text can't be rewritten`, LegacyWildcardSearchNote(`{"q":"part_1","page":1}`))
	assert.Empty(t, LegacyWildcardSearchNote(`{"q":"part-1"}`))
	assert.Empty(t, LegacyWildcardSearchNote(`{"page":1}`))
	assert.Empty(t, LegacyWildcardSearchNote(""))
	assert.Empty(t, LegacyWildcardSearchNote("{"))
}

func TestConvertLegacyWildcardDefaultFilters(t *testing.T) {
	in := `{"theme":"dark","defaultFilters":{` +
		`"scenes":{"mode":"SCENES","find_filter":{"q":"x_y","per_page":40},"object_filter":{"title":{"value":"part_1","modifier":"INCLUDES"},"url":{"value":"a_","modifier":"EXCLUDES"}},"ui_options":{"display_mode":0}},` +
		`"scene_markers":{"object_filter":{"scene_filter":{"title":{"value":"m%","modifier":"EQUALS"}}}},` +
		`"tags":{"mode":"TAGS","object_filter":{"name":{"value":"plain","modifier":"EQUALS"}}}}}`
	out, changed, notes, err := ConvertLegacyWildcardDefaultFilters(LegacySQLite, in)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, []string{
		`default filter for scenes: url: "excludes" on a field with several values can't be rewritten as a regular expression`,
		`default filter for scenes: search text "x_y": _ and % in it now match only themselves, and search text can't be rewritten`,
	}, notes)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, "dark", got["theme"])
	defaults := got["defaultFilters"].(map[string]interface{})
	scenes := defaults["scenes"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{
		"title": map[string]interface{}{"value": "(?is)(part.1)", "modifier": "MATCHES_REGEX"},
		"url":   map[string]interface{}{"value": "a_", "modifier": "EXCLUDES"},
	}, scenes["object_filter"])
	assert.Equal(t, map[string]interface{}{"q": "x_y", "per_page": float64(40)}, scenes["find_filter"])
	markers := defaults["scene_markers"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{
		"scene_filter": map[string]interface{}{"title": map[string]interface{}{"value": "(?is)^m.*$", "modifier": "MATCHES_REGEX"}},
	}, markers["object_filter"])
	tags := defaults["tags"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{"name": map[string]interface{}{"value": "plain", "modifier": "EQUALS"}}, tags["object_filter"])

	t.Run("nothing to change", func(t *testing.T) {
		for _, in := range []string{"", `{"theme":"dark"}`, `{"defaultFilters":{"scenes":{"object_filter":{"title":{"value":"x","modifier":"EQUALS"}}}}}`} {
			out, changed, notes, err := ConvertLegacyWildcardDefaultFilters(LegacySQLite, in)
			require.NoError(t, err)
			assert.False(t, changed)
			assert.Empty(t, notes)
			assert.Equal(t, in, out)
		}
	})
}
