package studio

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

// parentFinder finds studios by id, and stops a walk that doesn't end.
type parentFinder struct {
	parents map[int]int // studio id -> parent id, 0 for none
	finds   int
}

var errTooManyFinds = errors.New("the parent walk doesn't end")

func (f *parentFinder) Find(ctx context.Context, id int) (*models.Studio, error) {
	f.finds++
	if f.finds > 100 {
		return nil, errTooManyFinds
	}
	p, ok := f.parents[id]
	if !ok {
		return nil, nil
	}
	s := &models.Studio{ID: id}
	if p != 0 {
		s.ParentID = &p
	}
	return s, nil
}

func (f *parentFinder) FindMany(ctx context.Context, ids []int) ([]*models.Studio, error) {
	return nil, fmt.Errorf("not used")
}

func TestValidateParent(t *testing.T) {
	// 2 -> 1, and 4 and 5 are each other's parent: a loop saved before
	parents := map[int]int{1: 0, 2: 1, 3: 0, 4: 5, 5: 4}

	tests := []struct {
		name     string
		studioID int
		parentID int
		want     error
	}{
		{"no parent above", 3, 1, nil},
		{"parent chain", 3, 2, nil},
		{"own parent", 3, 3, ErrStudioOwnAncestor},
		{"own ancestor", 1, 2, ErrStudioOwnAncestor},
		// #2217: the walk ended only when the stack ran out
		{"loop above the new parent", 3, 4, nil},
		{"loop through the studio", 4, 5, ErrStudioOwnAncestor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &parentFinder{parents: parents}
			err := validateParent(testCtx, tt.studioID, tt.parentID, f)
			assert.NotErrorIs(t, err, errTooManyFinds)
			assert.Equal(t, tt.want, err)
		})
	}
}
