package models

import (
	"context"
	"slices"
)

// LibraryLimit is the part of the library an account may see: everything in the
// folders, tags and studios listed, including their sub-folders, sub-tags and
// sub-studios. A limit that lists nothing allows nothing.
type LibraryLimit struct {
	FolderIDs []int `json:"folder_ids"`
	TagIDs    []int `json:"tag_ids"`
	StudioIDs []int `json:"studio_ids"`
}

// Normalise sorts the IDs and removes duplicates.
func (l *LibraryLimit) Normalise() {
	for _, ids := range []*[]int{&l.FolderIDs, &l.TagIDs, &l.StudioIDs} {
		slices.Sort(*ids)
		*ids = slices.Compact(*ids)
	}
}

type libraryLimitCtxKey struct{}

// WithLibraryLimit returns a context in which reads are limited by l. A nil limit
// allows the whole library.
func WithLibraryLimit(ctx context.Context, l *LibraryLimit) context.Context {
	return context.WithValue(ctx, libraryLimitCtxKey{}, l)
}

// LibraryLimitFromContext returns the limit on reads in ctx, or nil if there isn't one.
func LibraryLimitFromContext(ctx context.Context) *LibraryLimit {
	l, _ := ctx.Value(libraryLimitCtxKey{}).(*LibraryLimit)
	return l
}
