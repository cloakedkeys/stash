//go:build integration

package database_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

// A page past the last result has no items, but the total is still the number
// of items the query finds, on both databases.
func TestQueryTotalPastLastPage(t *testing.T) {
	const items = 3
	const perPage = 2

	for _, tt := range stableOrderCases() {
		for _, sort := range tt.sorts {
			runWithRollbackTxn(t, fmt.Sprintf("%s sort %q", tt.name, sort), func(t *testing.T, ctx context.Context) {
				for i := 0; i < items; i++ {
					tt.create(t, ctx, i)
				}

				find := func(page int) *models.FindFilterType {
					pp := perPage
					f := &models.FindFilterType{Page: &page, PerPage: &pp}
					if sort != "" {
						s := sort
						f.Sort = &s
					}
					return f
				}

				for _, page := range []int{1, 2, 3, 5} {
					ids, count := tt.query(t, ctx, find(page))
					wantIDs := items - (page-1)*perPage
					wantIDs = max(0, min(perPage, wantIDs))
					assert.Len(t, ids, wantIDs, "items on page %d", page)
					assert.Equal(t, items, count, "total on page %d", page)
				}
			})
		}
	}
}
