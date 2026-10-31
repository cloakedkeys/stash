package scene

import (
	"context"
	"fmt"

	"github.com/stashapp/stash/pkg/models"
)

func (i *Importer) populateVoiceActors(ctx context.Context) error {
	i.voiceActorIDs = []int{}
	for _, name := range i.Input.VoiceActors {
		if i.VoiceActorWriter == nil {
			return fmt.Errorf("voice actor repository is required")
		}
		actor, err := i.VoiceActorWriter.FindByName(ctx, name)
		if err != nil {
			return err
		}
		if actor == nil {
			switch i.MissingRefBehaviour {
			case models.ImportMissingRefEnumIgnore:
				continue
			case models.ImportMissingRefEnumCreate:
				actor = &models.VoiceActor{Name: name}
				if err := i.VoiceActorWriter.Create(ctx, actor); err != nil {
					return err
				}
			default:
				return fmt.Errorf("scene voice actor %q not found", name)
			}
		}
		i.voiceActorIDs = append(i.voiceActorIDs, actor.ID)
	}
	return nil
}
