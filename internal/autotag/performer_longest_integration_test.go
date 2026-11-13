//go:build integration

package autotag

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/match"
	"github.com/stashapp/stash/pkg/models"
)

// Runs unchanged on SQLite and PostgreSQL, using rollback-only fixtures.
func TestPerformerDirections_LongestMatch(t *testing.T) {
	for _, tc := range []struct {
		filename string
		names    []string
		want     []bool
	}{
		{"Jane Doe.mp4", []string{"Jane", "Jane Doe"}, []bool{false, true}},
		{"Jane Doe.mp4", []string{"Jane Doe", "Doe"}, []bool{true, false}},
		{"Jane Doe Smith.mp4", []string{"Jane Doe", "Doe Smith"}, []bool{true, true}},
		{"Jane Alice.mp4", []string{"Jane", "Alice"}, []bool{true, true}},
		{"Jane Jane.mp4", []string{"Jane"}, []bool{true}},
		{"Jane Doe Jane.mp4", []string{"Jane", "Jane Doe"}, []bool{true, true}},
		{"Jane.-Doe.mp4", []string{"Jane", "Jane Doe", "Doe"}, []bool{false, true, false}},
	} {
		t.Run(tc.filename+"/"+tc.names[0], func(t *testing.T) {
			rollback := errors.New("rollback longest match fixtures")
			err := withTxn(func(ctx context.Context) error {
				var performers []*models.Performer
				for _, name := range tc.names {
					p := &models.Performer{Name: name}
					if err := r.Performer.Create(ctx, &models.CreatePerformerInput{Performer: p}); err != nil {
						return err
					}
					performers = append(performers, p)
				}
				path := filepath.Join("performer-longest", tc.filename)
				f, err := createSceneFile(ctx, path, r.Folder, r.File)
				if err != nil {
					return err
				}
				s := &models.Scene{}
				if err := createScene(ctx, r.Scene, s, f); err != nil {
					return err
				}
				cache := &match.Cache{}
				if err := cache.PreloadPerformers(ctx, r.Performer); err != nil {
					return err
				}
				for _, mode := range []struct {
					name  string
					cache *match.Cache
				}{{"SQL fallback", nil}, {"preloaded", cache}} {
					fromPath := make(map[int]bool)
					ft := getSceneFileTagger(s, mode.cache)
					ft.longestMatchOnly = true
					if err := ft.tagPerformers(ctx, r.Performer, func(_, id int) (bool, error) { fromPath[id] = true; return false, nil }); err != nil {
						return err
					}
					for i, p := range performers {
						fromPerformer := false
						pt := getPerformerTaggers(p, mode.cache)[0]
						pt.longestMatchOnly = true
						pt.performerReader = r.Performer
						if err := pt.tagScenes(ctx, nil, r.Scene, func(candidate *models.Scene) (bool, error) {
							if candidate.ID == s.ID {
								fromPerformer = true
							}
							return false, nil
						}); err != nil {
							return err
						}
						if fromPath[p.ID] != tc.want[i] || fromPerformer != tc.want[i] {
							t.Errorf("%s %s: file direction %v, performer direction %v, want %v", mode.name, p.Name, fromPath[p.ID], fromPerformer, tc.want[i])
						}
					}
				}
				return rollback
			})
			if !errors.Is(err, rollback) {
				t.Fatal(err)
			}
		})
	}
}
