package match

import (
	"reflect"
	"testing"

	"github.com/stashapp/stash/pkg/models"
)

func TestPerformersForPathSpans(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		want [][2]int
	}{
		{"Jane Doe", "Jane Doe", [][2]int{{0, 8}}},
		{"Jane Doe", "/Jane.-_ Doe.mp4", [][2]int{{1, 11}}},
		{"Jane Doe", "/JaneDoe.mp4", [][2]int{{1, 7}}},
		{"Jane", "/Jane.Jane.mp4", [][2]int{{1, 4}, {6, 4}}},
		{"Jane", "/Janet.mp4", nil},
		{"伏字", "/伏字.mp4", [][2]int{{1, 6}}},
		{"Jane", "/İ/JANE.mp4", [][2]int{{4, 4}}},
		{"İpek", "/İPEK.mp4", [][2]int{{1, 5}}},
		{"Ⱥna", "/Ⱥna.mp4", [][2]int{{1, 4}}},
		// Owner decision for TAG-1: skip one-letter names in both directions.
		{"Q", "/Q.mp4", nil},
		{"伏", "/伏.mp4", nil},
	} {
		t.Run(tc.name+tc.path, func(t *testing.T) {
			p := &models.Performer{ID: 1, Name: tc.name}
			for _, cache := range []*Cache{nil, {}} {
				var got [][2]int
				for _, m := range PerformersForPath(tc.path, []*models.Performer{p}, cache) {
					if m.Performer != p {
						t.Fatal("match lost performer identity")
					}
					got = append(got, [2]int{m.Position, m.Length})
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("got spans %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestPerformersForPathKeepsOverlappingNames(t *testing.T) {
	performers := []*models.Performer{{ID: 1, Name: "Jane"}, {ID: 2, Name: "Jane Doe"}}
	got := PerformersForPath("/Jane Doe.mp4", performers, nil)
	want := []PerformerMatch{{performers[0], 1, 4}, {performers[1], 1, 8}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPerformersForPathDoesNotMatchAliases(t *testing.T) {
	p := &models.Performer{ID: 1, Name: "Jane Doe", Aliases: models.NewRelatedStrings([]string{"Alice Smith"})}
	if got := PerformersForPath("/Alice Smith.mp4", []*models.Performer{p}, nil); len(got) != 0 {
		t.Fatalf("matched disabled alias: %v", got)
	}
}
