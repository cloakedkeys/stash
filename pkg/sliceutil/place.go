package sliceutil

// KeyPositions maps each key to every position at which it appears in keys,
// in order. Pass the result to PlaceByKey.
func KeyPositions[K comparable](keys []K) map[K][]int {
	ret := make(map[K][]int, len(keys))
	for i, k := range keys {
		ret[k] = append(ret[k], i)
	}
	return ret
}

// PlaceByKey writes each value in vs into dst at every position of its key,
// as given by positions from KeyPositions, so that dst lines up with the keys
// the positions were made from. A key that appears more than once gets the
// same value at each of its positions. Positions whose key has no value in vs
// are left unchanged, and values whose key has no position are ignored.
//
// It takes time in proportion to len(vs) plus the positions filled, so it can
// be called once per batch of values without rescanning the keys.
func PlaceByKey[T any, K comparable](dst []T, positions map[K][]int, vs []T, key func(T) K) {
	for _, v := range vs {
		for _, i := range positions[key(v)] {
			dst[i] = v
		}
	}
}

// FillRepeats copies, for every key that appears more than once in keys, the
// value at the key's first position in vs to each of its later positions.
// Use it after filling vs by each key's first position only.
//
// vs must be at least as long as keys.
func FillRepeats[T any, K comparable](vs []T, keys []K) {
	first := make(map[K]int, len(keys))
	for i, k := range keys {
		if j, ok := first[k]; ok {
			vs[i] = vs[j]
		} else {
			first[k] = i
		}
	}
}
