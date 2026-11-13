package match

import (
	"context"
	"unicode"
	"unicode/utf8"

	"github.com/stashapp/stash/pkg/models"
)

// PerformerMatch identifies a name occurrence. Position and Length are byte
// offsets in the original path, excluding surrounding word boundaries.
type PerformerMatch struct {
	Performer *models.Performer
	Position  int
	Length    int
}

// PerformersForPath matches candidate performers' names against a path.
// It returns occurrences, including overlapping performers, without selecting
// the longest name. Aliases and dates are not considered.
func PerformersForPath(path string, performers []*models.Performer, cache *Cache) []PerformerMatch {
	pm := newPathMatcher(path, cache)
	var ret []PerformerMatch
	for _, p := range performers {
		// Owner decision for TAG-1: both directions skip one-letter names.
		if utf8.RuneCountInString(p.Name) <= 1 {
			continue
		}
		re := cache.nameRegexp(p.Name, pm.useUnicode)
		for offset := 0; offset < len(pm.loweredPath); {
			found := re.FindStringSubmatchIndex(pm.loweredPath[offset:])
			if found == nil {
				break
			}
			start, end := offset+found[2], offset+found[3]
			position, endPosition := start, end
			if pm.useUnicode {
				position = originalPathOffset(path, start)
				endPosition = originalPathOffset(path, end)
			}
			ret = append(ret, PerformerMatch{
				Performer: p,
				Position:  position,
				Length:    endPosition - position,
			})
			// Reuse the trailing boundary so adjacent occurrences are found.
			// The previous match guarantees a boundary at this position.
			if end <= offset {
				// Names containing only spaces can match an empty string.
				_, size := utf8.DecodeRuneInString(pm.loweredPath[offset:])
				offset += size
			} else {
				offset = end
			}
		}
	}
	return ret
}

// strings.ToLower can change UTF-8 byte lengths (for example, İ becomes i).
// Translate offsets back to the caller's original path.
func originalPathOffset(path string, loweredOffset int) int {
	lowered := 0
	for original, r := range path {
		if lowered == loweredOffset {
			return original
		}
		lowered += utf8.RuneLen(unicode.ToLower(r))
	}
	return len(path)
}

// LongestPerformerMatches drops occurrences fully contained in a strictly longer
// occurrence. Equal spans, partial overlaps and separate occurrences survive.
func LongestPerformerMatches(matches []PerformerMatch) []PerformerMatch {
	var ret []PerformerMatch
	for _, m := range matches {
		contained := false
		for _, other := range matches {
			if other.Length > m.Length && other.Position <= m.Position && other.Position+other.Length >= m.Position+m.Length {
				contained = true
				break
			}
		}
		if !contained {
			ret = append(ret, m)
		}
	}
	return ret
}

// PathToLongestPerformers uses the same candidates as PathToPerformers, then
// removes contained occurrences before deduplicating performers.
func PathToLongestPerformers(ctx context.Context, path string, reader models.PerformerAutoTagQueryer, cache *Cache, trimExt bool) ([]*models.Performer, error) {
	performers, err := PathToPerformers(ctx, path, reader, cache, trimExt)
	if err != nil {
		return nil, err
	}
	var ret []*models.Performer
	seen := make(map[int]bool)
	for _, m := range LongestPerformerMatches(PerformersForPath(path, performers, cache)) {
		if !seen[m.Performer.ID] {
			seen[m.Performer.ID] = true
			ret = append(ret, m.Performer)
		}
	}
	return ret, nil
}
