package api

import (
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
)

// Requests written for v0.32.1 must still be accepted. GraphQL checks the
// variable's type name, so renaming an input type refuses them even when the
// fields still exist.
func TestUserDataUpdateAcceptsV0321Requests(t *testing.T) {
	schema := NewExecutableSchema(Config{Resolvers: newResolver(mocks.NewDatabase())}).Schema()

	valid := func(t *testing.T, query string) {
		t.Helper()
		_, errs := gqlparser.LoadQuery(schema, query)
		require.Empty(t, errs, query)
	}

	// the types each mutation took in v0.32.1
	v0321 := map[string]string{
		"sceneUserDataUpdate":     "UserRatingInput",
		"audioUserDataUpdate":     "UserRatingInput",
		"imageUserDataUpdate":     "UserRatingInput",
		"galleryUserDataUpdate":   "UserRatingInput",
		"groupUserDataUpdate":     "UserRatingInput",
		"performerUserDataUpdate": "UserRatingFavoriteInput",
		"studioUserDataUpdate":    "UserRatingFavoriteInput",
		"tagUserDataUpdate":       "UserRatingFavoriteInput",
	}

	for mutation, inputType := range v0321 {
		t.Run(mutation, func(t *testing.T) {
			valid(t, fmt.Sprintf(`mutation($input: %s!) { %s(input: $input) { id } }`, inputType, mutation))
			valid(t, fmt.Sprintf(`mutation { %s(input: {id: "1", rating100: 80}) { id } }`, mutation))
			// favourites are set through the same call
			valid(t, fmt.Sprintf(`mutation { %s(input: {id: "1", favorite: true}) { id } }`, mutation))
		})
	}

	valid(t, `mutation($input: StoryUserRatingInput!) { storyUserDataUpdate(input: $input) { id } }`)
	_, errs := gqlparser.LoadQuery(schema, `mutation { storyUserDataUpdate(input: {id: "1", favorite: true}) { id } }`)
	require.NotEmpty(t, errs, "stories have no favourite")
}
