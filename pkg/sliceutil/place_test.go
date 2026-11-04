package sliceutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type keyed struct {
	id   int
	name string
}

func keyedID(v *keyed) int { return v.id }

func TestKeyPositions(t *testing.T) {
	assert.Equal(t, map[int][]int{1: {0, 2, 3}, 2: {1}}, KeyPositions([]int{1, 2, 1, 1}))
	assert.Empty(t, KeyPositions[int](nil))
}

func TestPlaceByKey(t *testing.T) {
	a := &keyed{1, "a"}
	b := &keyed{2, "b"}
	c := &keyed{3, "c"}

	tests := []struct {
		name string
		keys []int
		vs   []*keyed
		want []*keyed
	}{
		{"in order", []int{1, 2}, []*keyed{a, b}, []*keyed{a, b}},
		{"out of order", []int{2, 1}, []*keyed{a, b}, []*keyed{b, a}},
		{"repeated", []int{1, 2, 1, 1}, []*keyed{b, a}, []*keyed{a, b, a, a}},
		{"missing", []int{1, 3, 1}, []*keyed{a}, []*keyed{a, nil, a}},
		{"none found", []int{1, 1}, nil, []*keyed{nil, nil}},
		{"value not asked for", []int{1}, []*keyed{c, a}, []*keyed{a}},
		{"no keys", nil, []*keyed{a}, []*keyed{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := make([]*keyed, len(tt.keys))
			PlaceByKey(got, KeyPositions(tt.keys), tt.vs, keyedID)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPlaceByKeyFillsInBatches(t *testing.T) {
	a := &keyed{1, "a"}
	b := &keyed{2, "b"}
	keys := []int{1, 2, 1}
	positions := KeyPositions(keys)

	got := make([]*keyed, len(keys))
	PlaceByKey(got, positions, []*keyed{a}, keyedID)
	PlaceByKey(got, positions, []*keyed{b}, keyedID)

	assert.Equal(t, []*keyed{a, b, a}, got)
}

func TestFillRepeats(t *testing.T) {
	tests := []struct {
		name string
		keys []int
		vs   []string
		want []string
	}{
		{"no repeats", []int{1, 2}, []string{"a", "b"}, []string{"a", "b"}},
		{"repeated", []int{1, 2, 1, 2, 1}, []string{"a", "b", "", "", ""}, []string{"a", "b", "a", "b", "a"}},
		{"empty first", []int{1, 1}, []string{"", "stale"}, []string{"", ""}},
		{"no keys", nil, []string{}, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			FillRepeats(tt.vs, tt.keys)
			assert.Equal(t, tt.want, tt.vs)
		})
	}
}

// BenchmarkPlaceByKey slots 30,000 results, found in reverse order and in
// batches of 1,000 as the stores do.
func BenchmarkPlaceByKey(b *testing.B) {
	const n, batch = 30000, 1000
	keys := make([]int, n)
	found := make([]*keyed, n)
	for i := range keys {
		keys[i] = i + 1
		found[n-1-i] = &keyed{id: i + 1}
	}

	for b.Loop() {
		dst := make([]*keyed, n)
		positions := KeyPositions(keys)
		for start := 0; start < n; start += batch {
			PlaceByKey(dst, positions, found[start:start+batch], keyedID)
		}
	}
}
