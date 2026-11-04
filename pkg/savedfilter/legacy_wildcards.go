package savedfilter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

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
//   - matching ignored case. \ was a plain character (PostgreSQL used
//     ESCAPE '', SQLite has no escape character by default).

// HasLegacyWildcard reports whether s has a character that LIKE used to treat
// as a wildcard.
func HasLegacyWildcard(s string) bool {
	return strings.ContainsAny(s, "_%")
}

// likeToRegex turns a LIKE pattern into the regular expression for it: % is any
// run of characters, _ is one character, and everything else is itself.
func likeToRegex(s string) string {
	var b strings.Builder
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '%':
			b.WriteString(regexp.QuoteMeta(s[start:i]))
			b.WriteString(".*")
			start = i + 1
		case '_':
			b.WriteString(regexp.QuoteMeta(s[start:i]))
			b.WriteString(".")
			start = i + 1
		}
	}
	b.WriteString(regexp.QuoteMeta(s[start:]))
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
func ConvertLegacyWildcardCriterion(modifier models.CriterionModifier, value string) (newModifier models.CriterionModifier, newValue string, ok bool) {
	if !HasLegacyWildcard(value) {
		return modifier, value, false
	}

	switch modifier {
	case models.CriterionModifierIncludes, models.CriterionModifierExcludes:
		terms := legacySearchTerms(value)
		for i, t := range terms {
			terms[i] = likeToRegex(t)
		}
		newValue = "(?is)(" + strings.Join(terms, "|") + ")"
	case models.CriterionModifierEquals, models.CriterionModifierNotEquals:
		newValue = "(?is)^" + likeToRegex(value) + "$"
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
func convertObjectFilter(t reflect.Type, filter map[string]interface{}, path string, notes *[]string) bool {
	changed := false
	for _, name := range sortedKeys(filter) {
		sub, isMap := filter[name].(map[string]interface{})
		if !isMap {
			continue
		}
		f, found := findField(t, name)
		if !found {
			continue
		}

		if f.Type == stringCriterionType {
			if convertCriterion(t, f, sub, path+name, notes) {
				changed = true
			}
			continue
		}

		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			if convertObjectFilter(ft, sub, path+name+".", notes) {
				changed = true
			}
		}
	}
	return changed
}

func convertCriterion(owner reflect.Type, f reflect.StructField, criterion map[string]interface{}, path string, notes *[]string) bool {
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

	newModifier, newValue, ok := ConvertLegacyWildcardCriterion(modifier, value)
	if !ok {
		return false
	}
	criterion["modifier"] = string(newModifier)
	criterion["value"] = newValue
	return true
}

// ConvertLegacyWildcards rewrites, in place, the text criteria of a saved
// object filter for the given mode that used _ or % as wildcards. It returns
// whether anything changed, and a note for each criterion it left as it was
// that now finds something different.
func ConvertLegacyWildcards(mode models.FilterMode, objectFilter map[string]interface{}) (changed bool, notes []string) {
	t, ok := filterTypes[mode]
	if !ok || objectFilter == nil {
		return false, nil
	}
	changed = convertObjectFilter(t, objectFilter, "", &notes)
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
func ConvertLegacyWildcardsJSON(mode models.FilterMode, objectFilter string) (newObjectFilter string, changed bool, notes []string, err error) {
	if strings.TrimSpace(objectFilter) == "" {
		return objectFilter, false, nil, nil
	}
	m, err := decodeJSONObject(objectFilter)
	if err != nil || m == nil {
		return objectFilter, false, nil, err
	}
	changed, notes = ConvertLegacyWildcards(mode, m)
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
func ConvertLegacyWildcardDefaultFilters(uiConfig string) (newUIConfig string, changed bool, notes []string, err error) {
	if strings.TrimSpace(uiConfig) == "" {
		return uiConfig, false, nil, nil
	}
	m, err := decodeJSONObject(uiConfig)
	if err != nil || m == nil {
		return uiConfig, false, nil, err
	}
	changed, notes = ConvertLegacyWildcardDefaultFiltersMap(m)
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
func ConvertLegacyWildcardDefaultFiltersMap(uiConfig map[string]interface{}) (changed bool, notes []string) {
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
			c, n := ConvertLegacyWildcards(models.FilterMode(mode), objectFilter)
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
