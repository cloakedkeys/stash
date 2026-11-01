package identify

import (
	"context"
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

type uniquePerformerCreator struct {
	performers []*models.Performer
}

func (c *uniquePerformerCreator) Create(_ context.Context, input *models.CreatePerformerInput) error {
	p := input.Performer
	for _, existing := range c.performers {
		if existing.Name == p.Name && existing.Disambiguation == p.Disambiguation {
			return fmt.Errorf("UNIQUE constraint failed: performers.name, performers.disambiguation")
		}
	}
	p.ID = len(c.performers) + 1
	c.performers = append(c.performers, p)
	return nil
}

func (c *uniquePerformerCreator) UpdateImage(context.Context, int, []byte) error {
	return nil
}

func TestPerformersDuplicateNewName(t *testing.T) {
	for _, tt := range []struct {
		name           string
		secondName     string
		disambiguation string
		wantIDs        []int
	}{
		{name: "duplicate", secondName: "New Performer", wantIDs: []int{1}},
		{name: "surrounding whitespace", secondName: " \tNew Performer\r\n\u00a0", wantIDs: []int{1}},
		{name: "different case", secondName: "NEW PERFORMER", wantIDs: []int{1, 2}},
		{name: "different disambiguation", secondName: "New Performer", disambiguation: "other", wantIDs: []int{1, 2}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			creator := &uniquePerformerCreator{}
			createMissing := true
			r := relationshipResolver{
				performerCreator: creator,
				fieldOptions: map[string]*FieldOptions{
					"performers": {Strategy: FieldStrategyMerge, CreateMissing: &createMissing},
				},
			}
			name := "New Performer"
			scraped := []*models.ScrapedPerformer{
				{Name: &name},
				{Name: &tt.secondName, Disambiguation: &tt.disambiguation},
			}
			ids, err := r.performers(context.Background(), nil, scraped, nil)
			require.NoError(t, err)
			require.Equal(t, tt.wantIDs, ids)
			require.Len(t, creator.performers, len(tt.wantIDs))
		})
	}
}
