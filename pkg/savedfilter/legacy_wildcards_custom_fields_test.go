package savedfilter

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// likeMatchEscaped is LIKE with \ as the escape character, as PostgreSQL's
// NOT ILIKE without an ESCAPE clause read it: \ makes the next character match
// only itself. ILIKE ignored case for every letter.
func likeMatchEscaped(pattern, s string) bool {
	type token struct {
		c       rune
		literal bool
	}
	var p []token
	pr := []rune(strings.ToLower(pattern))
	for i := 0; i < len(pr); i++ {
		if pr[i] == '\\' && i+1 < len(pr) {
			i++
			p = append(p, token{pr[i], true})
			continue
		}
		p = append(p, token{pr[i], false})
	}
	r := []rune(strings.ToLower(s))
	var match func(i, j int) bool
	match = func(i, j int) bool {
		if i == len(p) {
			return j == len(r)
		}
		switch {
		case !p[i].literal && p[i].c == '%':
			for k := j; k <= len(r); k++ {
				if match(i+1, k) {
					return true
				}
			}
			return false
		case !p[i].literal && p[i].c == '_':
			return j < len(r) && match(i+1, j+1)
		default:
			return j < len(r) && p[i].c == r[j] && match(i+1, j+1)
		}
	}
	return match(0, 0)
}

// oldCustomFieldMatch is whether a stored value s matched one value of a
// custom field includes or excludes criterion before.
func oldCustomFieldMatch(dialect LegacyDialect, modifier models.CriterionModifier, value, s string) bool {
	if modifier == includes {
		return dialectLikeMatch(dialect, "%"+value+"%", s)
	}
	if dialect == LegacyPostgres {
		return !likeMatchEscaped("%"+value+"%", s)
	}
	return !dialectLikeMatch(dialect, "%"+value+"%", s)
}

func TestConvertLegacyCustomFieldValueMatchesTheSame(t *testing.T) {
	values := []string{
		"site_a", "SITE_A", "site%", "%a", "_ite", "s%a", "site a", "100%", "a.b_", "(x)_",
		`C:\v_`, `a\_b`, `a\%`, `a\\b`, `a\`, `\`, `x\y`, "line_one%end", "%", "_", "__",
		"été_a", "ÉTÉ", "été",
	}
	texts := []string{
		"", "site_a", "site-a", "site a", "Site-A", "sites", "site", "100% real", "100 real", "a.b!", "axb!",
		"(x)y", `C:\v1`, `C:v1`, "a_b", "axb", `a\_b`, `a\xb`, "a%", "xa%", "a%x", `a\b`, `a\\b`, "ab",
		`a\`, `\`, "xy", `x\y`, "line-one\nmiddle\nend", "x",
		"ÉTÉ-A", "été-a", "Été_a",
	}
	for _, dialect := range []LegacyDialect{LegacySQLite, LegacyPostgres} {
		for _, modifier := range []models.CriterionModifier{includes, excludes} {
			for _, value := range values {
				newValue, ok := ConvertLegacyCustomFieldValue(dialect, modifier, value)
				if !ok {
					// no change: the value must have meant itself
					newValue = "(?is)" + likeToRegex(dialect, value)
				}
				re := regexp.MustCompile(newValue)
				for _, s := range texts {
					got := re.MatchString(s)
					if modifier == excludes {
						got = !got
					}
					assert.Equalf(t, oldCustomFieldMatch(dialect, modifier, value, s), got,
						"dialect %d %s %q on %q (regex %q)", dialect, modifier, value, s, newValue)
				}
			}
		}
	}
}

func TestConvertLegacyCustomFieldValue(t *testing.T) {
	tests := []struct {
		name     string
		dialect  LegacyDialect
		modifier models.CriterionModifier
		value    string
		want     string
		ok       bool
	}{
		{"includes", LegacySQLite, includes, "site_a", "(?is)site.a", true},
		{"includes, no words split", LegacySQLite, includes, "a_ b%", "(?is)a. b.*", true},
		{"includes no wildcard", LegacySQLite, includes, "site", "site", false},
		{"sqlite case only for A to Z", LegacySQLite, includes, "été_a", "(?is)(?-i:é)t(?-i:é).a", true},
		{"postgres case for every letter", LegacyPostgres, includes, "été_a", "(?is)été.a", true},
		{"includes backslash on postgres", LegacyPostgres, includes, `a\_b`, `(?is)a\\.b`, true},
		{"excludes", LegacySQLite, excludes, "site_a", "(?is)site.a", true},
		{"excludes backslash on sqlite is plain", LegacySQLite, excludes, `a\b`, `a\b`, false},
		{"excludes on postgres", LegacyPostgres, excludes, "site_a", "(?is)^.*site.a.*$", true},
		{"excludes escaped wildcard on postgres", LegacyPostgres, excludes, `a\_b`, "(?is)^.*a_b.*$", true},
		{"excludes backslash only on postgres", LegacyPostgres, excludes, `a\b`, "(?is)^.*ab.*$", true},
		{"excludes trailing backslash on postgres", LegacyPostgres, excludes, `a\`, "(?is)^.*a%$", true},
		{"excludes no wildcard on postgres", LegacyPostgres, excludes, "site", "site", false},
		{"equals uses no LIKE", LegacySQLite, equals, "site_a", "site_a", false},
		{"not equals uses no LIKE", LegacyPostgres, notEquals, "site_a", "site_a", false},
		{"already a regex", LegacySQLite, matchesRegex, "site_a", "site_a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ConvertLegacyCustomFieldValue(tt.dialect, tt.modifier, tt.value)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
			_, err := regexp.Compile(got)
			assert.NoError(t, err)
		})
	}
}

func TestConvertLegacyWildcardsCustomFields(t *testing.T) {
	tests := []struct {
		name    string
		dialect LegacyDialect
		mode    models.FilterMode
		in      string
		want    string
		changed bool
		notes   []string
	}{
		{
			name: "includes, every value as a regex",
			mode: models.FilterModeScenes,
			in:   `{"custom_fields":[{"field":"source","value":["site_a","x"],"modifier":"INCLUDES"}]}`,
			want: `{"custom_fields":[{"field":"source","modifier":"MATCHES_REGEX","value":["(?is)site.a","(?is)x"]}]}`, changed: true,
		},
		{
			name: "excludes",
			mode: models.FilterModePerformers,
			in:   `{"custom_fields":[{"field":"source","value":["a%b"],"modifier":"EXCLUDES"}]}`,
			want: `{"custom_fields":[{"field":"source","modifier":"NOT_MATCHES_REGEX","value":["(?is)a.*b"]}]}`, changed: true,
		},
		{
			name:    "excludes with \\ on postgres",
			dialect: LegacyPostgres,
			mode:    models.FilterModeScenes,
			in:      `{"custom_fields":[{"field":"source","value":["a\\b"],"modifier":"EXCLUDES"}]}`,
			want:    `{"custom_fields":[{"field":"source","modifier":"NOT_MATCHES_REGEX","value":["(?is)^.*ab.*$"]}]}`, changed: true,
		},
		{
			name: "excludes with \\ on sqlite is unchanged",
			mode: models.FilterModeScenes,
			in:   `{"custom_fields":[{"field":"source","value":["a\\b"],"modifier":"EXCLUDES"}]}`,
			want: `{"custom_fields":[{"field":"source","value":["a\\b"],"modifier":"EXCLUDES"}]}`,
		},
		{
			name: "equals, not equals and other criteria untouched",
			mode: models.FilterModeScenes,
			in: `{"custom_fields":[{"field":"a","value":["x_y"],"modifier":"EQUALS"},{"field":"b","value":["x_y"],"modifier":"NOT_EQUALS"},` +
				`{"field":"c","value":["x"],"modifier":"INCLUDES"},{"field":"d","value":[1,5],"modifier":"BETWEEN"},{"field":"e","value":["x_y"],"modifier":"MATCHES_REGEX"}]}`,
			want: `{"custom_fields":[{"field":"a","value":["x_y"],"modifier":"EQUALS"},{"field":"b","value":["x_y"],"modifier":"NOT_EQUALS"},` +
				`{"field":"c","value":["x"],"modifier":"INCLUDES"},{"field":"d","value":[1,5],"modifier":"BETWEEN"},{"field":"e","value":["x_y"],"modifier":"MATCHES_REGEX"}]}`,
		},
		{
			name: "only the criteria that need it",
			mode: models.FilterModeTags,
			in:   `{"custom_fields":[{"field":"a","value":["x"],"modifier":"INCLUDES"},{"field":"b","value":["x_"],"modifier":"INCLUDES"}]}`,
			want: `{"custom_fields":[{"field":"a","value":["x"],"modifier":"INCLUDES"},{"field":"b","modifier":"MATCHES_REGEX","value":["(?is)x."]}]}`, changed: true,
		},
		{
			name:  "a number among the values is left with a note",
			mode:  models.FilterModeScenes,
			in:    `{"custom_fields":[{"field":"n","value":["1_",2],"modifier":"INCLUDES"}]}`,
			want:  `{"custom_fields":[{"field":"n","value":["1_",2],"modifier":"INCLUDES"}]}`,
			notes: []string{`custom_fields[0] (n): values that aren't text are mixed with text that used _ or %, which can't be rewritten as regular expressions`},
		},
		{
			name: "a number alone needs nothing",
			mode: models.FilterModeScenes,
			in:   `{"custom_fields":[{"field":"n","value":[2],"modifier":"INCLUDES"}]}`,
			want: `{"custom_fields":[{"field":"n","value":[2],"modifier":"INCLUDES"}]}`,
		},
		{
			name: "a single value that isn't a list",
			mode: models.FilterModeScenes,
			in:   `{"custom_fields":[{"field":"source","value":"site_a","modifier":"INCLUDES"}]}`,
			want: `{"custom_fields":[{"field":"source","modifier":"MATCHES_REGEX","value":"(?is)site.a"}]}`, changed: true,
		},
		{
			name: "in a sub-filter",
			mode: models.FilterModeScenes,
			in:   `{"performers_filter":{"custom_fields":[{"field":"source","value":["site_a"],"modifier":"INCLUDES"}]}}`,
			want: `{"performers_filter":{"custom_fields":[{"field":"source","modifier":"MATCHES_REGEX","value":["(?is)site.a"]}]}}`, changed: true,
		},
		{
			name: "every mode that has custom fields",
			mode: models.FilterModeGroups,
			in:   `{"custom_fields":[{"field":"source","value":["site_a"],"modifier":"INCLUDES"}]}`,
			want: `{"custom_fields":[{"field":"source","modifier":"MATCHES_REGEX","value":["(?is)site.a"]}]}`, changed: true,
		},
		{
			name: "with other criteria",
			mode: models.FilterModeScenes,
			in:   `{"title":{"value":"a_b","modifier":"INCLUDES"},"custom_fields":[{"field":"source","value":["a_b"],"modifier":"INCLUDES"}]}`,
			want: `{"custom_fields":[{"field":"source","modifier":"MATCHES_REGEX","value":["(?is)a.b"]}],"title":{"modifier":"MATCHES_REGEX","value":"(?is)(a.b)"}}`, changed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := decode(t, tt.in)
			changed, notes := ConvertLegacyWildcardsFor(tt.dialect, tt.mode, m)
			assert.Equal(t, tt.changed, changed)
			assert.Equal(t, tt.notes, notes)
			assert.JSONEq(t, tt.want, encode(t, m))
		})
	}
}

// ConvertLegacyWildcards, which the import uses, reads filters as SQLite did.
func TestConvertLegacyWildcardsCustomFieldsDefaultsToSQLite(t *testing.T) {
	m := decode(t, `{"custom_fields":[{"field":"source","value":["a\\b"],"modifier":"EXCLUDES"}]}`)
	changed, _ := ConvertLegacyWildcards(models.FilterModeScenes, m)
	assert.False(t, changed)
}

func TestConvertLegacyWildcardDefaultFiltersCustomFields(t *testing.T) {
	in := `{"defaultFilters":{"scenes":{"mode":"SCENES","object_filter":{"custom_fields":[{"field":"s","value":["a\\b"],"modifier":"EXCLUDES"}]}}}}`
	out, changed, _, err := ConvertLegacyWildcardDefaultFilters(LegacyPostgres, in)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.JSONEq(t, `{"defaultFilters":{"scenes":{"mode":"SCENES","object_filter":{"custom_fields":[{"field":"s","modifier":"NOT_MATCHES_REGEX","value":["(?is)^.*ab.*$"]}]}}}}`, out)

	_, changed, _, err = ConvertLegacyWildcardDefaultFilters(LegacySQLite, in)
	require.NoError(t, err)
	assert.False(t, changed)
}
