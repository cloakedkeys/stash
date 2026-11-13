package match

import (
	"github.com/stashapp/stash/pkg/models"
	"reflect"
	"testing"
)

func TestLongestPerformerMatches(t *testing.T) {
	for _, tc := range []struct {
		path  string
		names []string
		want  []string
	}{
		{"/Jane Doe.mp4", []string{"Jane", "Jane Doe", "Doe"}, []string{"Jane Doe"}},
		{"/Jane Doe Smith.mp4", []string{"Jane Doe", "Doe Smith"}, []string{"Jane Doe", "Doe Smith"}},
		{"/Jane Alice.mp4", []string{"Jane", "Alice"}, []string{"Jane", "Alice"}},
		{"/Jane Jane.mp4", []string{"Jane"}, []string{"Jane", "Jane"}},
		{"/Jane Doe Jane.mp4", []string{"Jane", "Jane Doe"}, []string{"Jane", "Jane Doe"}},
		{"/Jane Doe.mp4", []string{"Jane Doe", "Jane Doe"}, []string{"Jane Doe", "Jane Doe"}},
		{"/?/Jane.-Doe.mp4", []string{"Jane", "Jane Doe", "Doe"}, []string{"Jane Doe"}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			var performers []*models.Performer
			for i, name := range tc.names {
				performers = append(performers, &models.Performer{ID: i + 1, Name: name})
			}
			var got []string
			for _, m := range LongestPerformerMatches(PerformersForPath(tc.path, performers, nil)) {
				got = append(got, m.Performer.Name)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
