package manager

import (
	"testing"

	"github.com/stashapp/stash/pkg/models"
)

func TestQueueImagePhashesIncludesVideoFiles(t *testing.T) {
	for _, tt := range []struct {
		name      string
		enabled   bool
		hasPhash  bool
		overwrite bool
		want      int
	}{
		{name: "missing phashes", enabled: true, want: 2},
		{name: "existing phashes", enabled: true, hasPhash: true},
		{name: "overwrite phashes", enabled: true, hasPhash: true, overwrite: true, want: 2},
		{name: "disabled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := []models.File{
				&models.ImageFile{BaseFile: &models.BaseFile{ID: 1, Path: "image.png"}},
				&models.VideoFile{BaseFile: &models.BaseFile{ID: 2, Path: "clip.mp4"}},
			}
			if tt.hasPhash {
				for _, f := range files {
					f.Base().SetFingerprint(models.Fingerprint{
						Type:        models.FingerprintTypePhash,
						Fingerprint: int64(123),
					})
				}
			}
			i := &models.Image{Files: models.NewRelatedFiles(files)}
			j := &GenerateJob{
				input:     GenerateMetadataInput{ImagePhashes: tt.enabled},
				overwrite: tt.overwrite,
			}
			queue := make(chan Task, len(files))
			j.queueImageJob(nil, i, queue)
			close(queue)

			if len(queue) != tt.want {
				t.Fatalf("queued %d tasks, want %d", len(queue), tt.want)
			}
			if j.totals.imagePhashes != int64(tt.want) || j.totals.tasks != tt.want {
				t.Fatalf("totals = %+v, want %d image phashes and tasks", j.totals, tt.want)
			}
			index := 0
			for task := range queue {
				phashTask, ok := task.(*GenerateImagePhashTask)
				if !ok {
					t.Fatalf("queued %T, want *GenerateImagePhashTask", task)
				}
				if phashTask.File != files[index] || phashTask.Overwrite != tt.overwrite {
					t.Errorf("task %d has the wrong file or overwrite flag", index)
				}
				if got := phashTask.GetDescription(); got != "Generating phash for "+files[index].Base().Path {
					t.Errorf("description = %q", got)
				}
				index++
			}
		})
	}
}
