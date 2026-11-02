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
	// GetText returns the text stored for a story without a text file, or nil.
	GetText(ctx context.Context, id int) (*string, error)
	HasText(ctx context.Context, id int) (bool, error)
}
type StoryWriter interface {
	Create(ctx context.Context, object *Story, fileIDs []FileID) error
	Update(ctx context.Context, object *Story) error
	UpdatePartial(ctx context.Context, id int, partial StoryPartial) (*Story, error)
	Destroy(ctx context.Context, id int) error
	// DestroyIfEmpty removes a story with no title and no files, and reports whether it did.
	DestroyIfEmpty(ctx context.Context, id int) (bool, error)
	AddFileID(ctx context.Context, id int, fileID FileID) error
	AssignFiles(ctx context.Context, id int, fileIDs []FileID) error
	// UpdateText stores a story's text. An empty text removes it.
	UpdateText(ctx context.Context, id int, text string) error
}
type StoryReaderWriter interface {
	StoryReader
	StoryWriter
}
