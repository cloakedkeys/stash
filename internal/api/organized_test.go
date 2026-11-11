package api

import (
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestOrganizedRequiresAdmin(t *testing.T) {
	for _, mutation := range []string{
		`performerCreate(input:{name:"New",organized:true}) { id }`,
		`performerUpdate(input:{id:"1",organized:false}) { id }`,
		`bulkPerformerUpdate(input:{ids:["1","2"],organized:true}) { id }`,
		`tagCreate(input:{name:"New",organized:true}) { id }`,
		`tagUpdate(input:{id:"1",organized:false}) { id }`,
		`bulkTagUpdate(input:{ids:["1","2"],organized:true}) { id }`,
	} {
		for _, role := range []models.RoleEnum{models.RoleEnumRead, models.RoleEnumModify} {
			t.Run(string(role)+"/"+mutation, func(t *testing.T) {
				db := mocks.NewDatabase()
				result := performerImageQuery(t, newTestGraphQLServer(newResolver(db)), "mutation { "+mutation+" }", role)
				require.Contains(t, result, "unauthorized")
				require.NotContains(t, result, "is not defined")
				db.AssertExpectations(t)
			})
		}
	}
}

func TestBulkOrganized(t *testing.T) {
	for _, value := range []bool{true, false} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			t.Run("performers", func(t *testing.T) {
				db := mocks.NewDatabase()
				for _, id := range []int{1, 2} {
					db.Performer.On("Find", mock.Anything, id).Return(&models.Performer{ID: id, Organized: value}, nil).Twice()
					db.Performer.On("GetAliases", mock.Anything, id).Return([]string{}, nil).Once()
					db.Performer.On("UpdatePartial", mock.Anything, id, mock.MatchedBy(func(p models.PerformerPartial) bool {
						return p.Organized == models.NewOptionalBool(value)
					})).Return(&models.Performer{ID: id, Organized: value}, nil).Once()
				}
				query := fmt.Sprintf(`mutation { bulkPerformerUpdate(input:{ids:["1","2"],organized:%t}) { id organized } }`, value)
				result := performerImageQuery(t, newTestGraphQLServer(newResolver(db)), query, models.RoleEnumAdmin)
				require.NotContains(t, result, "errors")
				require.Contains(t, result, fmt.Sprintf(`"organized":%t`, value))
				db.AssertExpectations(t)
			})
			t.Run("tags", func(t *testing.T) {
				db := mocks.NewDatabase()
				for _, id := range []int{1, 2} {
					db.Tag.On("Find", mock.Anything, id).Return(&models.Tag{ID: id, Organized: value}, nil).Twice()
					db.Tag.On("UpdatePartial", mock.Anything, id, mock.MatchedBy(func(p models.TagPartial) bool {
						return p.Organized == models.NewOptionalBool(value)
					})).Return(&models.Tag{ID: id, Organized: value}, nil).Once()
				}
				query := fmt.Sprintf(`mutation { bulkTagUpdate(input:{ids:["1","2"],organized:%t}) { id organized } }`, value)
				result := performerImageQuery(t, newTestGraphQLServer(newResolver(db)), query, models.RoleEnumAdmin)
				require.NotContains(t, result, "errors")
				require.Contains(t, result, fmt.Sprintf(`"organized":%t`, value))
				db.AssertExpectations(t)
			})
		})
	}
}

func TestMergeKeepsOrganized(t *testing.T) {
	for _, value := range []bool{true, false} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			for _, tc := range []struct {
				name      string
				values    string
				organized models.OptionalBool
			}{
				{name: "omitted", values: `id:"1"`},
				{name: "explicit override", values: fmt.Sprintf(`id:"1",organized:%t`, !value), organized: models.NewOptionalBool(!value)},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Run("performer", func(t *testing.T) {
						db := mocks.NewDatabase()
						kept := &models.Performer{ID: 1, Organized: value}
						db.Performer.On("Find", mock.Anything, 1).Return(kept, nil).Once()
						db.Performer.On("FindMany", mock.Anything, []int{2}).Return([]*models.Performer{{ID: 2, Organized: !value}}, nil).Once()
						db.Performer.On("UpdatePartial", mock.Anything, 1, mock.MatchedBy(func(p models.PerformerPartial) bool {
							return p.Organized == tc.organized
						})).Return(kept, nil).Once()
						db.Performer.On("Merge", mock.Anything, []int{2}, 1).Return(nil).Once()
						query := fmt.Sprintf(`mutation { performerMerge(input:{source:["2"],destination:"1",values:{%s}}) { organized } }`, tc.values)
						result := performerImageQuery(t, newTestGraphQLServer(newResolver(db)), query, models.RoleEnumAdmin)
						require.NotContains(t, result, "errors")
						if !tc.organized.Set {
							require.Contains(t, result, fmt.Sprintf(`"organized":%t`, value))
						}
						db.AssertExpectations(t)
					})
					t.Run("tag", func(t *testing.T) {
						db := mocks.NewDatabase()
						kept := &models.Tag{ID: 1, Organized: value}
						db.Tag.On("Find", mock.Anything, 1).Return(kept, nil).Twice()
						db.Tag.On("FindMany", mock.Anything, []int{2}).Return([]*models.Tag{{ID: 2, Organized: !value}}, nil).Once()
						db.Tag.On("UpdatePartial", mock.Anything, 1, mock.MatchedBy(func(p models.TagPartial) bool {
							return p.Organized == tc.organized
						})).Return(kept, nil).Once()
						db.Tag.On("Merge", mock.Anything, []int{2}, 1).Return(nil).Once()
						query := fmt.Sprintf(`mutation { tagsMerge(input:{source:["2"],destination:"1",values:{%s}}) { organized } }`, tc.values)
						result := performerImageQuery(t, newTestGraphQLServer(newResolver(db)), query, models.RoleEnumAdmin)
						require.NotContains(t, result, "errors")
						if !tc.organized.Set {
							require.Contains(t, result, fmt.Sprintf(`"organized":%t`, value))
						}
						db.AssertExpectations(t)
					})
				})
			}
		})
	}
}
