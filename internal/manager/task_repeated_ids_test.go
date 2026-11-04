package manager

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Export and generate handle each item once, even if its id is listed more
// than once or written differently (#2078).
func TestTaskIDsRepeated(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		want []int
	}{
		{"distinct", []string{"3", "1", "2"}, []int{3, 1, 2}},
		{"repeated", []string{"1", "2", "1", "2"}, []int{1, 2}},
		{"written differently", []string{"1", "01"}, []int{1}},
		{"none", nil, []int{}},
	}

	for _, tt := range tests {
		t.Run("export "+tt.name, func(t *testing.T) {
			spec := newExportSpec(&ExportObjectTypeInput{Ids: tt.ids})
			assert.Equal(t, tt.want, spec.IDs)
		})

		t.Run("generate "+tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, generateIDs(tt.ids))
		})
	}
}

// Tasks that take ids as text keep each id once, in order. Other values,
// such as "*" or ids that aren't numbers, are kept as written (#2092).
func TestUniqueIDStrings(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		want []string
	}{
		{"distinct", []string{"3", "1", "2"}, []string{"3", "1", "2"}},
		{"repeated", []string{"1", "2", "1", "2"}, []string{"1", "2"}},
		{"written differently", []string{"01", "1"}, []string{"01"}},
		{"wildcard", []string{"*", "1", "*"}, []string{"*", "1"}},
		{"not numbers", []string{"x", "1", "x", "y"}, []string{"x", "1", "y"}},
		{"none", nil, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, uniqueIDStrings(tt.ids))
		})
	}
}

// Auto tag looks up and tags each performer, studio and tag once, even if
// its id is listed more than once or written differently (#2092).
func TestAutoTagRepeatedIDs(t *testing.T) {
	db := mocks.NewDatabase()
	// skipped items still show that each id was looked up once
	db.Performer.On("Find", mock.Anything, 1).Return(&models.Performer{ID: 1, IgnoreAutoTag: true}, nil)
	db.Performer.On("Find", mock.Anything, 2).Return(&models.Performer{ID: 2, IgnoreAutoTag: true}, nil)
	db.Studio.On("Find", mock.Anything, 3).Return(&models.Studio{ID: 3, IgnoreAutoTag: true}, nil)
	db.Tag.On("Find", mock.Anything, 4).Return(&models.Tag{ID: 4, IgnoreAutoTag: true}, nil)

	j := &autoTagJob{
		repository: db.Repository(),
		input: AutoTagMetadataInput{
			Performers: []string{"1", "2", "01", "1"},
			Studios:    []string{"3", "3"},
			Tags:       []string{"4", "004"},
		},
	}

	require.NoError(t, j.Execute(context.Background(), &job.Progress{}))

	db.Performer.AssertNumberOfCalls(t, "Find", 2)
	db.Performer.AssertCalled(t, "Find", mock.Anything, 1)
	db.Performer.AssertCalled(t, "Find", mock.Anything, 2)
	db.Studio.AssertNumberOfCalls(t, "Find", 1)
	db.Tag.AssertNumberOfCalls(t, "Find", 1)
}

// Stash-box batch tagging by id makes one task per item, even if its id is
// listed more than once or written differently (#2092).
func TestStashBoxBatchTagRepeatedIDs(t *testing.T) {
	box := &models.StashBox{Endpoint: "https://stashbox.example/graphql"}
	ctx := context.Background()

	t.Run("performers", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.Performer.On("Find", mock.Anything, 1).Return(&models.Performer{ID: 1}, nil)
		db.Performer.On("Find", mock.Anything, 2).Return(&models.Performer{ID: 2}, nil)
		db.Performer.On("GetStashIDs", mock.Anything, mock.Anything).Return(nil, nil)
		s := &Manager{Repository: db.Repository()}

		tasks, err := s.batchTagPerformersByIds(ctx, StashBoxBatchTagInput{Ids: []string{"1", "2", "01", "1"}}, box)
		require.NoError(t, err)
		require.Len(t, tasks, 2)
		assert.Equal(t, 1, tasks[0].(*stashBoxBatchPerformerTagTask).performer.ID)
		assert.Equal(t, 2, tasks[1].(*stashBoxBatchPerformerTagTask).performer.ID)
	})

	t.Run("performers by deprecated performer_ids", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.Performer.On("Find", mock.Anything, 1).Return(&models.Performer{ID: 1}, nil)
		db.Performer.On("GetStashIDs", mock.Anything, mock.Anything).Return(nil, nil)
		s := &Manager{Repository: db.Repository()}

		tasks, err := s.batchTagPerformersByIds(ctx, StashBoxBatchTagInput{PerformerIds: []string{"1", "1"}}, box)
		require.NoError(t, err)
		assert.Len(t, tasks, 1)
	})

	t.Run("studios", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.Studio.On("Find", mock.Anything, 3).Return(&models.Studio{ID: 3}, nil)
		db.Studio.On("GetStashIDs", mock.Anything, mock.Anything).Return(nil, nil)
		s := &Manager{Repository: db.Repository()}

		tasks, err := s.batchTagStudiosByIds(ctx, StashBoxBatchTagInput{Ids: []string{"3", "03"}}, box)
		require.NoError(t, err)
		assert.Len(t, tasks, 1)
	})

	t.Run("tags", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.Tag.On("Find", mock.Anything, 4).Return(&models.Tag{ID: 4}, nil)
		db.Tag.On("GetStashIDs", mock.Anything, mock.Anything).Return(nil, nil)
		s := &Manager{Repository: db.Repository()}

		tasks, err := s.batchTagTagsByIds(ctx, StashBoxBatchTagInput{Ids: []string{"4", "4"}}, box)
		require.NoError(t, err)
		assert.Len(t, tasks, 1)
	})

	t.Run("distinct ids are all tagged", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.Tag.On("Find", mock.Anything, 4).Return(&models.Tag{ID: 4}, nil)
		db.Tag.On("Find", mock.Anything, 5).Return(&models.Tag{ID: 5}, nil)
		db.Tag.On("GetStashIDs", mock.Anything, mock.Anything).Return(nil, nil)
		s := &Manager{Repository: db.Repository()}

		tasks, err := s.batchTagTagsByIds(ctx, StashBoxBatchTagInput{Ids: []string{"4", "x", "5"}}, box)
		require.NoError(t, err)
		assert.Len(t, tasks, 2)
	})
}
