package models

import (
	"context"
	"time"
)

// VoiceActor represents a person credited for voice work, independently of
// on-screen performers. Scene credits never affect performer relationships.
type VoiceActor struct {
	ID        int       `json:"-"`
	Name      string    `json:"name"`
	Details   string    `json:"details"`
	Aliases   []string  `json:"aliases"`
	URLs      []string  `json:"urls"`
	Favorite  bool      `json:"favorite"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type VoiceActorCreateInput struct {
	Name     string
	Details  *string
	Aliases  []string
	URLs     []string
	Favorite *bool
}

type VoiceActorUpdateInput struct {
	ID       string
	Name     *string
	Details  *string
	Aliases  []string
	URLs     []string
	Favorite *bool
}

type FindVoiceActorsResult struct {
	Count       int
	VoiceActors []*VoiceActor
}

type VoiceActorReaderWriter interface {
	Find(context.Context, int) (*VoiceActor, error)
	FindByName(context.Context, string) (*VoiceActor, error)
	Query(context.Context, *FindFilterType, *bool) ([]*VoiceActor, int, error)
	Create(context.Context, *VoiceActor) error
	Update(context.Context, *VoiceActor) error
	Destroy(context.Context, int) error
	FindBySceneID(context.Context, int) ([]*VoiceActor, error)
	SceneIDs(context.Context, int) ([]int, error)
	SetScene(context.Context, int, []int) error
	MergeScenes(context.Context, int, []int) error
}
