package database

import (
	"context"
	"time"

	"github.com/stashapp/stash/pkg/models"
)

type SceneStore interface {
	SetCustomFields(ctx context.Context, id int, values models.CustomFieldsInput) error
	models.SceneReaderWriter

	CountByStudioID(ctx context.Context, studioID int) (int, error)
	blobJoinQueryBuilder
	oDateManager
	viewDateManager
}

type blobJoinQueryBuilder interface {
	DestroyImage(ctx context.Context, id int, blobCol string) error
	GetImage(ctx context.Context, id int, blobCol string) ([]byte, error)
	HasImage(ctx context.Context, id int, blobCol string) (bool, error)
	UpdateImage(ctx context.Context, id int, blobCol string, image []byte) error
}

type oDateManager interface {
	AddO(ctx context.Context, id int, dates []time.Time) ([]time.Time, error)
	DeleteO(ctx context.Context, id int, dates []time.Time) ([]time.Time, error)
	GetAllOCount(ctx context.Context) (int, error)
	GetManyOCount(ctx context.Context, ids []int) ([]int, error)
	GetManyODates(ctx context.Context, ids []int) ([][]time.Time, error)
	GetOCount(ctx context.Context, id int) (int, error)
	GetODates(ctx context.Context, id int) ([]time.Time, error)
	GetUniqueOCount(ctx context.Context) (int, error)
	ResetO(ctx context.Context, id int) (int, error)
}

type viewDateManager interface {
	AddViews(ctx context.Context, id int, dates []time.Time) ([]time.Time, error)
	CountAllViews(ctx context.Context) (int, error)
	CountUniqueViews(ctx context.Context) (int, error)
	CountViews(ctx context.Context, id int) (int, error)
	DeleteAllViews(ctx context.Context, id int) (int, error)
	DeleteViews(ctx context.Context, id int, dates []time.Time) ([]time.Time, error)
	GetManyLastViewed(ctx context.Context, ids []int) ([]*time.Time, error)
	GetManyViewCount(ctx context.Context, ids []int) ([]int, error)
	GetManyViewDates(ctx context.Context, ids []int) ([][]time.Time, error)
	GetViewDates(ctx context.Context, id int) ([]time.Time, error)
	LastView(ctx context.Context, id int) (*time.Time, error)
}
