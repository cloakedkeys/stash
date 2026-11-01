package models

import "context"

type StoryGetter interface {
	Find(ctx context.Context, id int) (*Story, error)
	FindMany(ctx context.Context, ids []int) ([]*Story, error)
	FindByIDs(ctx context.Context, ids []int) ([]*Story, error)
}
type StoryReader interface {
	StoryGetter
	URLLoader
	TagIDLoader
	FileIDLoader
	IDsFromFileIDsLoader
	FindByFileID(ctx context.Context, id FileID) ([]*Story, error)
	FindByPrimaryFileID(ctx context.Context, id FileID) ([]*Story, error)
	GetFiles(ctx context.Context, id int) ([]File, error)
	Count(ctx context.Context) (int, error)
	CountByFileID(ctx context.Context, id FileID) (int, error)
	Query(ctx context.Context, options StoryQueryOptions) (*StoryQueryResult, error)
	QueryCount(ctx context.Context, filter *StoryFilterType, find *FindFilterType) (int, error)
}
type StoryWriter interface {
	Create(ctx context.Context, object *Story, fileIDs []FileID) error
	Update(ctx context.Context, object *Story) error
	UpdatePartial(ctx context.Context, id int, partial StoryPartial) (*Story, error)
	Destroy(ctx context.Context, id int) error
	AddFileID(ctx context.Context, id int, fileID FileID) error
	AssignFiles(ctx context.Context, id int, fileIDs []FileID) error
}
type StoryReaderWriter interface {
	StoryReader
	StoryWriter
}
