package manager

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const batchTagTestEndpoint = "https://stashbox.example/graphql"

// batchTagMissingID is an id with no matching item, as when an item is
// deleted between the user selecting it and the batch tag job running.
const batchTagMissingID = 999

func batchTagTestStashIDs(id int) []models.StashID {
	// item 3 is already linked to the endpoint, the others are not
	if id == 3 {
		return []models.StashID{{Endpoint: batchTagTestEndpoint, StashID: "remote-3"}}
	}
	return []models.StashID{}
}

func setupBatchTagTestDB(existing []int) *mocks.Database {
	db := mocks.NewDatabase()

	for _, id := range existing {
		db.Performer.On("Find", mock.Anything, id).Return(&models.Performer{ID: id, Name: "performer"}, nil)
		db.Performer.On("GetStashIDs", mock.Anything, id).Return(batchTagTestStashIDs(id), nil)
		db.Studio.On("Find", mock.Anything, id).Return(&models.Studio{ID: id, Name: "studio"}, nil)
		db.Studio.On("GetStashIDs", mock.Anything, id).Return(batchTagTestStashIDs(id), nil)
		db.Tag.On("Find", mock.Anything, id).Return(&models.Tag{ID: id, Name: "tag"}, nil)
		db.Tag.On("GetStashIDs", mock.Anything, id).Return(batchTagTestStashIDs(id), nil)
	}

	db.Performer.On("Find", mock.Anything, batchTagMissingID).Return(nil, nil)
	db.Studio.On("Find", mock.Anything, batchTagMissingID).Return(nil, nil)
	db.Tag.On("Find", mock.Anything, batchTagMissingID).Return(nil, nil)

	return db
}

func batchTagTaskIDs(t *testing.T, tasks []Task) []int {
	t.Helper()

	var ids []int
	for _, task := range tasks {
		switch tt := task.(type) {
		case *stashBoxBatchPerformerTagTask:
			ids = append(ids, tt.performer.ID)
		case *stashBoxBatchStudioTagTask:
			ids = append(ids, tt.studio.ID)
		case *stashBoxBatchTagTagTask:
			ids = append(ids, tt.tag.ID)
		default:
			t.Fatalf("unexpected task type %T", task)
		}
	}
	return ids
}

type batchTagByIdsFn func(*Manager, context.Context, StashBoxBatchTagInput, *models.StashBox) ([]Task, error)

var batchTagByIdsFns = []struct {
	name string
	fn   batchTagByIdsFn
}{
	{"performers", (*Manager).batchTagPerformersByIds},
	{"studios", (*Manager).batchTagStudiosByIds},
	{"tags", (*Manager).batchTagTagsByIds},
}

func runBatchTagByIds(t *testing.T, fn batchTagByIdsFn, db *mocks.Database, input StashBoxBatchTagInput) []int {
	t.Helper()

	m := &Manager{Repository: db.Repository()}
	box := &models.StashBox{Endpoint: batchTagTestEndpoint}

	var tasks []Task
	var err error
	require.NotPanics(t, func() {
		tasks, err = fn(m, context.Background(), input, box)
	})
	require.NoError(t, err)

	return batchTagTaskIDs(t, tasks)
}

// A missing id must be skipped, not crash the job, and the rest of the
// batch must still be tagged.
func TestBatchTagByIds_MissingItemIsSkipped(t *testing.T) {
	for _, f := range batchTagByIdsFns {
		t.Run(f.name, func(t *testing.T) {
			db := setupBatchTagTestDB([]int{1, 2})
			input := StashBoxBatchTagInput{Ids: []string{"1", "999", "2"}}

			got := runBatchTagByIds(t, f.fn, db, input)
			assert.Equal(t, []int{1, 2}, got)
		})
	}
}

// Existing items are still picked the same way: refresh=false tags the
// items not yet linked to the endpoint, refresh=true the linked ones,
// non-numeric ids are ignored, and order is kept.
func TestBatchTagByIds_ExistingItemsUnchanged(t *testing.T) {
	tests := []struct {
		name    string
		refresh bool
		want    []int
	}{
		{"not refresh", false, []int{2, 1}},
		{"refresh", true, []int{3}},
	}

	for _, f := range batchTagByIdsFns {
		for _, tt := range tests {
			t.Run(f.name+"/"+tt.name, func(t *testing.T) {
				db := setupBatchTagTestDB([]int{1, 2, 3})
				input := StashBoxBatchTagInput{
					Ids:     []string{"2", "bad", "3", "1"},
					Refresh: tt.refresh,
				}

				got := runBatchTagByIds(t, f.fn, db, input)
				assert.Equal(t, tt.want, got)
			})
		}
	}
}

// The deprecated performer_ids field is still read when ids is empty.
func TestBatchTagPerformersByIds_DeprecatedPerformerIds(t *testing.T) {
	db := setupBatchTagTestDB([]int{1, 2})
	input := StashBoxBatchTagInput{PerformerIds: []string{"1", "999", "2"}} //nolint:staticcheck

	got := runBatchTagByIds(t, (*Manager).batchTagPerformersByIds, db, input)
	assert.Equal(t, []int{1, 2}, got)
}
