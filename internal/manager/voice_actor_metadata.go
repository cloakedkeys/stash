package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

func (t *ExportTask) exportVoiceActors(ctx context.Context) error {
	dir := filepath.Join(t.baseDir, "voice_actors")
	if err := fsutil.EnsureDir(dir); err != nil {
		return err
	}
	if err := fsutil.EmptyDir(dir); err != nil {
		return err
	}
	all := t.full || (t.voiceActors != nil && t.voiceActors.all)
	if !all && (t.voiceActors == nil || len(t.voiceActors.IDs) == 0) {
		return nil
	}
	perPage := -1
	actors, _, err := t.repository.VoiceActor.Query(ctx, &models.FindFilterType{PerPage: &perPage}, nil)
	if err != nil {
		return err
	}
	for _, actor := range actors {
		if !all && !slices.Contains(t.voiceActors.IDs, actor.ID) {
			continue
		}
		data, err := json.MarshalIndent(actor, "", "  ")
		if err != nil {
			return err
		}
		// Include the ID in the filename so different names that sanitise to the
		// same basename never overwrite one another. References use names.
		name := fmt.Sprintf("%s.%d.json", fsutil.SanitiseBasename(actor.Name), actor.ID)
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func (t *ImportTask) ImportVoiceActors(ctx context.Context) {
	dir := filepath.Join(t.BaseDir, "voice_actors")
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		logger.Errorf("[voice actors] %v", err)
		return
	}
	for _, entry := range files {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		err := t.repository.WithTxn(ctx, func(ctx context.Context) error {
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				return err
			}
			var actor models.VoiceActor
			if err := json.Unmarshal(data, &actor); err != nil {
				return err
			}
			existing, err := t.repository.VoiceActor.FindByName(ctx, actor.Name)
			if err != nil {
				return err
			}
			if existing == nil {
				return t.repository.VoiceActor.Create(ctx, &actor)
			}
			switch t.DuplicateBehaviour {
			case ImportDuplicateEnumIgnore:
				return nil
			case ImportDuplicateEnumOverwrite:
				actor.ID = existing.ID
				return t.repository.VoiceActor.Update(ctx, &actor)
			default:
				return fmt.Errorf("voice actor %q already exists", actor.Name)
			}
		})
		if err != nil {
			logger.Errorf("[voice actors] %s: %v", entry.Name(), err)
		}
	}
}
