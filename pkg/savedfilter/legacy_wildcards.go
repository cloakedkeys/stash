package savedfilter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/stashapp/stash/pkg/models"
)

// Text filters used to pass their value to LIKE, so _ matched any one
// character and % any run of characters. They now match both literally. The
// functions here rewrite saved filters that used them as wildcards into
// regular expressions that find the same things, so upgrading doesn't change
// what a saved filter finds. Both databases' migrations use them.
//
// Before the change, on both databases:
//   - includes split the value on whitespace and matched any of the words;
//     a value wrapped in double quotes was one phrase. Excludes matched none.
//   - equals and not equals compared the whole value.
//   - matching ignored case: on SQLite only for A to Z, on PostgreSQL (ILIKE)
//     for every letter.
//   - \ was a plain character (PostgreSQL used ESCAPE '', SQLite has no escape
//     character by default).
//
// Custom field criteria (custom_fields) also used LIKE, differently:
//   - includes matched objects whose value had every one of the values in it,
//     each as typed, without splitting on whitespace. Excludes matched objects
//     whose value had none of them, or that didn't have the field.
//   - equals and not equals didn't use LIKE.
//   - on PostgreSQL, excludes had no ESCAPE clause, so \ escaped the character
//     after it. Includes used ESCAPE '', and SQLite has no escape character.

// LegacyDialect is the database a saved filter was used with before the
// change, since their LIKE differed.
type LegacyDialect int

const (
	LegacySQLite LegacyDialect = iota
	LegacyPostgres
)

// HasLegacyWildcard reports whether s has a character that LIKE used to treat
// as a wildcard.
func HasLegacyWildcard(s string) bool {
	return strings.ContainsAny(s, "_%")
}

// literalRegex is the regular expression for the character r as LIKE matched
// it, inside a pattern that ignores case. SQLite's LIKE ignored case only for
// A to Z, so on SQLite a letter outside them that has another case matches
// only itself.
//
// (?i) also matches k and s to the Kelvin sign and the long s, which SQLite
// didn't; those are left, rather than making every k and s harder to read.
func literalRegex(dialect LegacyDialect, r rune) string {
	if dialect == LegacySQLite && r >= utf8.RuneSelf && unicode.SimpleFold(r) != r {
		return "(?-i:" + string(r) + ")"
	}
	return regexp.QuoteMeta(string(r))
}

// likeToRegex turns a LIKE pattern into the regular expression for it: % is any
// run of characters, _ is one character, and everything else is itself.
func likeToRegex(dialect LegacyDialect, s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '%':
			b.WriteString(".*")
		case r == '_':
			b.WriteString(".")
		case r == utf8.RuneError && size == 1:
			// not UTF-8: keep the byte
			b.WriteString(s[i : i+1])
		default:
			b.WriteString(literalRegex(dialect, r))
		}
		i += size
	}
	return b.String()
}

// likeEscapedToRegex is likeToRegex for a PostgreSQL LIKE pattern in which \
// makes the character after it match only itself, as in PostgreSQL's default.
// The pattern is matched against the whole value, so the result is anchored.
// PostgreSQL ignored case for every letter, so letters need nothing special.
func likeEscapedToRegex(s string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && i+1 < len(s):
			// the escaped character; the rest of a multi-byte character is
			// copied as it is by the loop
			i++
			b.WriteString(regexp.QuoteMeta(s[i : i+1]))
		case c == '%':
			b.WriteString(".*")
		case c == '_':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(s[i : i+1]))
		}
	}
	b.WriteString("$")
	return b.String()
}

var whitespace = regexp.MustCompile(`\s+`)

// legacySearchTerms splits an includes or excludes value the way the old
// search did: on whitespace, unless the value is in double quotes.
func legacySearchTerms(value string) []string {
	q := strings.TrimSpace(value)
	phrase := strings.Trim(q, `"`)
	if phrase != q {
		return []string{phrase}
	}
	return strings.Split(whitespace.ReplaceAllString(q, " "), " ")
}

// ConvertLegacyWildcardCriterion returns the regular expression modifier and
// value that match what a text criterion with the given modifier and value
// used to match. ok is false when the criterion needs no change: its value has
// no _ or %, or its modifier didn't use LIKE.
//
// (?i) ignores case and (?s) lets . match a line break, as LIKE's _ and % did.
func ConvertLegacyWildcardCriterion(dialect LegacyDialect, modifier models.CriterionModifier, value string) (newModifier models.CriterionModifier, newValue string, ok bool) {
	if !HasLegacyWildcard(value) {
		return modifier, value, false
	}

	switch modifier {
	case models.CriterionModifierIncludes, models.CriterionModifierExcludes:
		terms := legacySearchTerms(value)
		for i, t := range terms {
			terms[i] = likeToRegex(dialect, t)
		}
		newValue = "(?is)(" + strings.Join(terms, "|") + ")"
	case models.CriterionModifierEquals, models.CriterionModifierNotEquals:
		newValue = "(?is)^" + likeToRegex(dialect, value) + "$"
	default:
		return modifier, value, false
	}

	switch modifier {
	case models.CriterionModifierIncludes, models.CriterionModifierEquals:
		newModifier = models.CriterionModifierMatchesRegex
	default:
		newModifier = models.CriterionModifierNotMatchesRegex
	}

	return newModifier, newValue, true
}

// filterTypes are the object filter types of each saved filter mode.
var filterTypes = map[models.FilterMode]reflect.Type{
	models.FilterModeScenes:       reflect.TypeOf(models.SceneFilterType{}),
	models.FilterModePerformers:   reflect.TypeOf(models.PerformerFilterType{}),
	models.FilterModeStudios:      reflect.TypeOf(models.StudioFilterType{}),
	models.FilterModeGalleries:    reflect.TypeOf(models.GalleryFilterType{}),
	models.FilterModeSceneMarkers: reflect.TypeOf(models.SceneMarkerFilterType{}),
	models.FilterModeMovies:       reflect.TypeOf(models.GroupFilterType{}),
	models.FilterModeGroups:       reflect.TypeOf(models.GroupFilterType{}),
	models.FilterModeTags:         reflect.TypeOf(models.TagFilterType{}),
	models.FilterModeImages:       reflect.TypeOf(models.ImageFilterType{}),
	models.FilterModeAudios:       reflect.TypeOf(models.AudioFilterType{}),
	models.FilterModeStories:      reflect.TypeOf(models.StoryFilterType{}),
}

type fieldKey struct {
	owner reflect.Type
	field string
}

func key(v interface{}, field string) fieldKey {
	return fieldKey{reflect.TypeOf(v), field}
}

// listFields hold several values per object (stringListCriterionHandlerBuilder).
// "excludes" on them means no value contains the text, which a regex can't say:
// "doesn't match regex" means some value doesn't match.
var listFields = map[fieldKey]bool{
	key(models.AudioFilterType{}, "URL"):         true,
	key(models.GalleryFilterType{}, "URL"):       true,
	key(models.GroupFilterType{}, "URL"):         true,
	key(models.ImageFilterType{}, "URL"):         true,
	key(models.PerformerFilterType{}, "URL"):     true,
	key(models.SceneFilterType{}, "URL"):         true,
	key(models.StoryFilterType{}, "URL"):         true,
	key(models.StudioFilterType{}, "URL"):        true,
	key(models.GroupFilterType{}, "Aliases"):     true,
	key(models.PerformerFilterType{}, "Aliases"): true,
	key(models.StudioFilterType{}, "Aliases"):    true,
	key(models.TagFilterType{}, "Aliases"):       true,
}

// skipFields are text criteria whose value never went to LIKE as typed, so
// they're left alone.
var skipFields = map[fieldKey]string{
	// language codes; "excludes" compares the whole code rather than searching in it
	key(models.SceneFilterType{}, "Captions"):      "captions",
	key(models.VideoFileFilterInput{}, "Captions"): "captions",
	// a phash compared by distance
	key(models.SceneFilterType{}, "Phash"): "",
	// legacy numeric and year range filters
	key(models.PerformerFilterType{}, "Height"):       "",
	key(models.PerformerFilterType{}, "CareerLength"): "",
}

var stringCriterionType = reflect.TypeOf(&models.StringCriterionInput{})

// jsonFieldName returns the name encoding/json uses for a field, or "" if it
// isn't encoded.
func jsonFieldName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	if name, _, _ := strings.Cut(tag, ","); name != "" {
		return name
	}
	return f.Name
}

// findField finds the field a JSON key decodes into, matching as encoding/json
// does: exactly, then ignoring case.
func findField(t reflect.Type, name string) (reflect.StructField, bool) {
	var folded *reflect.StructField
	for _, f := range reflect.VisibleFields(t) {
		if f.Anonymous || !f.IsExported() {
			continue
		}
		jsonName := jsonFieldName(f)
		if jsonName == name {
			return f, true
		}
		if folded == nil && jsonName != "" && strings.EqualFold(jsonName, name) {
			f := f
			folded = &f
		}
	}
	if folded != nil {
		return *folded, true
	}
	return reflect.StructField{}, false
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// convertObjectFilter rewrites the text criteria of an object filter of type t
// in place, along with those of its sub-filters. It returns whether anything
// changed, and appends a note for each criterion left as it was that now finds
// something different.
func convertObjectFilter(dialect LegacyDialect, t reflect.Type, filter map[string]interface{}, path string, notes *[]string) bool {
	changed := false
	for _, name := range sortedKeys(filter) {
		f, found := findField(t, name)
		if !found {
			continue
		}

		if f.Type == customFieldsType {
			if list, ok := filter[name].([]interface{}); ok && convertCustomFields(dialect, list, path+name, notes) {
				changed = true
			}
			continue
		}

		sub, isMap := filter[name].(map[string]interface{})
		if !isMap {
			continue
		}

		if f.Type == stringCriterionType {
			if convertCriterion(dialect, t, f, sub, path+name, notes) {
				changed = true
			}
			continue
		}

		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			if convertObjectFilter(dialect, ft, sub, path+name+".", notes) {
				changed = true
			}
		}
	}
	return changed
}

func convertCriterion(dialect LegacyDialect, owner reflect.Type, f reflect.StructField, criterion map[string]interface{}, path string, notes *[]string) bool {
	value, ok := criterion["value"].(string)
	if !ok || !HasLegacyWildcard(value) {
		return false
	}
	modifierStr, _ := criterion["modifier"].(string)
	modifier := models.CriterionModifier(modifierStr)

	k := fieldKey{owner, f.Name}
	if what, skip := skipFields[k]; skip {
		if what != "" {
			*notes = append(*notes, fmt.Sprintf("%s: %s are left as they are", path, what))
		}
		return false
	}

	if listFields[k] && modifier == models.CriterionModifierExcludes {
		*notes = append(*notes, fmt.Sprintf("%s: \"excludes\" on a field with several values can't be rewritten as a regular expression", path))
		return false
	}

	newModifier, newValue, ok := ConvertLegacyWildcardCriterion(dialect, modifier, value)
	if !ok {
		return false
	}
	criterion["modifier"] = string(newModifier)
	criterion["value"] = newValue
	return true
}

var customFieldsType = reflect.TypeOf([]models.CustomFieldCriterionInput{})

// convertCustomFields rewrites the custom field criteria in list, each
// {"field", "value": [...], "modifier"}, in place.
func convertCustomFields(dialect LegacyDialect, list []interface{}, path string, notes *[]string) bool {
	changed := false
	for i, item := range list {
		criterion, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		field, _ := criterion["field"].(string)
		if convertCustomFieldCriterion(dialect, criterion, fmt.Sprintf("%s[%d] (%s)", path, i, field), notes) {
			changed = true
		}
	}
	return changed
}

// ConvertLegacyCustomFieldValue returns the regular expression that matches
// what a custom field includes or excludes value used to match: a value with
// the text in it. ok is false when the value needs no change.
func ConvertLegacyCustomFieldValue(dialect LegacyDialect, modifier models.CriterionModifier, value string) (newValue string, ok bool) {
	backslashEscapes := dialect == LegacyPostgres && modifier == models.CriterionModifierExcludes
	switch {
	case modifier != models.CriterionModifierIncludes && modifier != models.CriterionModifierExcludes:
		return value, false
	case backslashEscapes:
		if !HasLegacyWildcard(value) && !strings.Contains(value, `\`) {
			return value, false
		}
		// the value was put between two % and then read with \ as the escape
		// character, so a \ at its end escaped the last %
		return "(?is)" + likeEscapedToRegex("%"+value+"%"), true
	default:
		if !HasLegacyWildcard(value) {
			return value, false
		}
		return "(?is)" + likeToRegex(dialect, value), true
	}
}

func convertCustomFieldCriterion(dialect LegacyDialect, criterion map[string]interface{}, path string, notes *[]string) bool {
	modifierStr, _ := criterion["modifier"].(string)
	modifier := models.CriterionModifier(modifierStr)

	var values []interface{}
	single := false
	switch v := criterion["value"].(type) {
	case []interface{}:
		values = v
	case string:
		values = []interface{}{v}
		single = true
	default:
		return false
	}

	newValues := make([]interface{}, len(values))
	needed, allText := false, true
	for i, v := range values {
		s, isText := v.(string)
		if !isText {
			allText = false
			continue
		}
		newValue, ok := ConvertLegacyCustomFieldValue(dialect, modifier, s)
		if ok {
			needed = true
			newValues[i] = newValue
		} else {
			// a value with no wildcard, as a regular expression too, since all
			// the values share the modifier
			newValues[i] = "(?is)" + likeToRegex(dialect, s)
		}
	}
	if !needed {
		return false
	}
	if !allText {
		// a number can't go to a regular expression modifier
		*notes = append(*notes, fmt.Sprintf("%s: values that aren't text are mixed with text that used _ or %%, which can't be rewritten as regular expressions", path))
		return false
	}

	if modifier == models.CriterionModifierIncludes {
		criterion["modifier"] = string(models.CriterionModifierMatchesRegex)
	} else {
		criterion["modifier"] = string(models.CriterionModifierNotMatchesRegex)
	}
	if single {
		criterion["value"] = newValues[0]
	} else {
		criterion["value"] = newValues
	}
	return true
}

// ConvertLegacyWildcards rewrites, in place, the text criteria of a saved
// object filter for the given mode that used _ or % as wildcards, as they
// worked on SQLite. It returns whether anything changed, and a note for each
// criterion it left as it was that now finds something different.
func ConvertLegacyWildcards(mode models.FilterMode, objectFilter map[string]interface{}) (changed bool, notes []string) {
	return ConvertLegacyWildcardsFor(LegacySQLite, mode, objectFilter)
}

// ConvertLegacyWildcardsFor is ConvertLegacyWildcards for a filter used with
// the given database.
func ConvertLegacyWildcardsFor(dialect LegacyDialect, mode models.FilterMode, objectFilter map[string]interface{}) (changed bool, notes []string) {
	t, ok := filterTypes[mode]
	if !ok || objectFilter == nil {
		return false, nil
	}
	changed = convertObjectFilter(dialect, t, objectFilter, "", &notes)
	return changed, notes
}

func decodeJSONObject(s string) (map[string]interface{}, error) {
	d := json.NewDecoder(strings.NewReader(s))
	// keep numbers as they were written
	d.UseNumber()
	var ret map[string]interface{}
	if err := d.Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
}

func encodeJSON(v interface{}) (string, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// ConvertLegacyWildcardsJSON is ConvertLegacyWildcards for an object filter
// stored as JSON. It returns the new JSON when something changed.
func ConvertLegacyWildcardsJSON(dialect LegacyDialect, mode models.FilterMode, objectFilter string) (newObjectFilter string, changed bool, notes []string, err error) {
	if strings.TrimSpace(objectFilter) == "" {
		return objectFilter, false, nil, nil
	}
	m, err := decodeJSONObject(objectFilter)
	if err != nil || m == nil {
		return objectFilter, false, nil, err
	}
	changed, notes = ConvertLegacyWildcardsFor(dialect, mode, m)
	if !changed {
		return objectFilter, false, notes, nil
	}
	newObjectFilter, err = encodeJSON(m)
	if err != nil {
		return objectFilter, false, nil, err
	}
	return newObjectFilter, true, notes, nil
}

// LegacyWildcardSearchNote returns a note if the search text of a find filter
// stored as JSON has _ or %. Search text has no regex form, so it can't be
// rewritten.
func LegacyWildcardSearchNote(findFilter string) string {
	if strings.TrimSpace(findFilter) == "" {
		return ""
	}
	m, err := decodeJSONObject(findFilter)
	if err != nil {
		return ""
	}
	return searchNote(m)
}

func searchNote(findFilter map[string]interface{}) string {
	q, _ := findFilter["q"].(string)
	if !HasLegacyWildcard(q) {
		return ""
	}
	return fmt.Sprintf("search text %q: _ and %% in it now match only themselves, and search text can't be rewritten", q)
}

// ConvertLegacyWildcardDefaultFilters rewrites the default filters in a user's
// UI configuration, stored as JSON, as ConvertLegacyWildcards does. It returns
// the new JSON when something changed. Each note starts with the default
// filter it's about.
func ConvertLegacyWildcardDefaultFilters(dialect LegacyDialect, uiConfig string) (newUIConfig string, changed bool, notes []string, err error) {
	if strings.TrimSpace(uiConfig) == "" {
		return uiConfig, false, nil, nil
	}
	m, err := decodeJSONObject(uiConfig)
	if err != nil || m == nil {
		return uiConfig, false, nil, err
	}
	changed, notes = ConvertLegacyWildcardDefaultFiltersMap(dialect, m)
	if !changed {
		return uiConfig, false, notes, nil
	}
	newUIConfig, err = encodeJSON(m)
	if err != nil {
		return uiConfig, false, nil, err
	}
	return newUIConfig, true, notes, nil
}

// ConvertLegacyWildcardDefaultFiltersMap rewrites, in place, the default
// filters in a UI configuration, as ConvertLegacyWildcards does. Each note
// starts with the default filter it's about.
func ConvertLegacyWildcardDefaultFiltersMap(dialect LegacyDialect, uiConfig map[string]interface{}) (changed bool, notes []string) {
	defaults, _ := uiConfig["defaultFilters"].(map[string]interface{})
	for _, view := range sortedKeys(defaults) {
		entry, ok := defaults[view].(map[string]interface{})
		if !ok {
			continue
		}
		mode := strings.ToUpper(view)
		if s, ok := entry["mode"].(string); ok && s != "" {
			mode = s
		}
		prefix := "default filter for " + view + ": "
		if objectFilter, ok := entry["object_filter"].(map[string]interface{}); ok {
			c, n := ConvertLegacyWildcardsFor(dialect, models.FilterMode(mode), objectFilter)
			changed = changed || c
			for _, note := range n {
				notes = append(notes, prefix+note)
			}
		}
		if findFilter, ok := entry["find_filter"].(map[string]interface{}); ok {
			if note := searchNote(findFilter); note != "" {
				notes = append(notes, prefix+note)
			}
		}
	}
	return changed, notes
}
