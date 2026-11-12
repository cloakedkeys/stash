//go:build integration

package autotag

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/match"
	"github.com/stashapp/stash/pkg/models"
)

// This uses the shared SQLite/PostgreSQL suite and rolls back every fixture.
// It checks the existing SQL candidate query as well as the final matcher.
func TestPerformerDirections_Characterization(t *testing.T) {
	for _, tc := range []struct {
		name, filename string
		want           bool
	}{
		{"Jane Doe", "Jane Doe.mp4", true},
		{"Jane Doe", "Jane.Doe.mp4", true},
		{"Jane Doe", "Jane-Doe.mp4", true},
		{"Jane Doe", "Jane_Doe.mp4", true},
		{"Jane Doe", "jAnE_dOe.mp4", true},
		{"Jane", "Janet.mp4", false},
		{"Jane", "Jane Doe.mp4", true},
		{"Jane Doe", "JaneDoe.mp4", true},
		{"伏字", "伏字.mp4", true},
		{"Jane", "伏字Jane.mp4", false},
		// Owner decision for TAG-1: skip one-letter names in both directions.
		{"Q", "Q.mp4", false},
	} {
		t.Run(tc.name+"/"+tc.filename, func(t *testing.T) {
			rollback := errors.New("rollback characterization fixtures")
			err := withTxn(func(ctx context.Context) error {
				p := &models.Performer{Name: tc.name}
				if err := r.Performer.Create(ctx, &models.CreatePerformerInput{Performer: p}); err != nil {
					return err
				}
				path := filepath.Join("performer-characterization", tc.filename)
				f, err := createSceneFile(ctx, path, r.Folder, r.File)
				if err != nil {
					return err
				}
				s := &models.Scene{}
				if err := createScene(ctx, r.Scene, s, f); err != nil {
					return err
				}

				fromPerformer := false
				tt := getPerformerTaggers(p, nil)[0]
				if err := tt.tagScenes(ctx, nil, r.Scene, func(candidate *models.Scene) (bool, error) {
					if candidate.ID == s.ID {
						fromPerformer = true
					}
					return false, nil
				}); err != nil {
					return err
				}
				if fromPerformer != tc.want {
					t.Errorf("performer -> files: got %v, want %v", fromPerformer, tc.want)
				}

				cache := &match.Cache{}
				if err := cache.PreloadPerformers(ctx, r.Performer); err != nil {
					return err
				}
				for _, mode := range []struct {
					name  string
					cache *match.Cache
				}{{"SQL fallback", nil}, {"preloaded", cache}} {
					matches, err := match.PathToPerformers(ctx, path, r.Performer, mode.cache, false)
					if err != nil {
						return err
					}
					fromPath := false
					for _, found := range matches {
						fromPath = fromPath || found.ID == p.ID
					}
					if fromPath != fromPerformer {
						t.Errorf("%s: path -> performers = %v, performer -> files = %v", mode.name, fromPath, fromPerformer)
					}
				}
				return rollback
			})
			if !errors.Is(err, rollback) {
				t.Fatal(fmt.Errorf("characterization transaction: %w", err))
			}
		})
	}
}
