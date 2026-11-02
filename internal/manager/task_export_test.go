package manager

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/require"
)

type exportBackend struct {
	database.Database
	backend database.DatabaseType
}

func (d exportBackend) DatabaseBackend() database.DatabaseType { return d.backend }

// exportConnectionTags rejects overlapping reads like a PostgreSQL transaction.
// The first read waits for another worker so the old code reliably overlaps.
type exportConnectionTags struct {
	models.TagReaderWriter
	singleConnection bool
	active           atomic.Int32
	started          atomic.Bool
	overlap          chan struct{}
	once             sync.Once
}

func (s *exportConnectionTags) FindMany(_ context.Context, ids []int) ([]*models.Tag, error) {
	var tags []*models.Tag
	for _, id := range ids {
		tags = append(tags, &models.Tag{ID: id, Name: fmt.Sprintf("tag%d", id)})
	}
	return tags, nil
}

func (s *exportConnectionTags) GetAliases(_ context.Context, id int) ([]string, error) {
	active := s.active.Add(1)
	defer s.active.Add(-1)
	if active > 1 {
		s.once.Do(func() { close(s.overlap) })
		if s.singleConnection {
			return nil, errors.New("conn busy")
		}
	}
	if s.started.CompareAndSwap(false, true) {
		select {
		case <-s.overlap:
		case <-time.After(time.Second):
		}
	}
	return []string{fmt.Sprintf("alias%d", id)}, nil
}

func (*exportConnectionTags) GetStashIDs(context.Context, int) ([]models.StashID, error) {
	return nil, nil
}

func (*exportConnectionTags) GetImage(context.Context, int) ([]byte, error) { return nil, nil }

func (*exportConnectionTags) FindByChildTagID(context.Context, int) ([]*models.Tag, error) {
	return nil, nil
}

func (*exportConnectionTags) GetCustomFields(context.Context, int) (map[string]interface{}, error) {
	return nil, nil
}

func TestExportUsesOnePostgresConnection(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(2)
	t.Cleanup(func() { runtime.GOMAXPROCS(previousProcs) })

	for _, backend := range []database.DatabaseType{database.PostgresBackend, database.SqliteBackend} {
		t.Run(string(backend), func(t *testing.T) {
			store := &exportConnectionTags{
				singleConnection: backend == database.PostgresBackend,
				overlap:          make(chan struct{}),
			}
			generated := paths.NewPaths(t.TempDir(), "")
			previous := instance
			instance = &Manager{
				Database:      exportBackend{backend: backend},
				Paths:         &generated,
				DownloadStore: NewDownloadStore(),
				Repository: models.Repository{
					TxnManager: models.TxnManager{Manager: &metadataTxnManager{}},
					Tag:        store,
				},
			}
			t.Cleanup(func() { instance = previous })
			task := CreateExportTask(models.HashAlgorithmOshash, ExportObjectsInput{
				Tags: &ExportObjectTypeInput{Ids: []string{"1", "2"}},
			})
			var wg sync.WaitGroup
			wg.Add(1)
			task.Start(context.Background(), &wg)
			wg.Wait()

			select {
			case <-store.overlap:
				require.Equal(t, database.SqliteBackend, backend, "PostgreSQL transaction reads overlapped")
			default:
				require.Equal(t, database.PostgresBackend, backend, "SQLite should retain parallel exports")
			}
			require.NotEmpty(t, task.DownloadHash)
			archive, err := zip.OpenReader(instance.DownloadStore.m[task.DownloadHash].path)
			require.NoError(t, err)
			defer archive.Close()
			require.Len(t, archive.File, 2, "every selected item must be exported")
			aliases := make(map[string][]string)
			for _, file := range archive.File {
				r, err := file.Open()
				require.NoError(t, err)
				var document struct {
					Name    string   `json:"name"`
					Aliases []string `json:"aliases"`
				}
				err = json.NewDecoder(r).Decode(&document)
				r.Close()
				require.NoError(t, err)
				aliases[document.Name] = document.Aliases
			}
			require.Equal(t, map[string][]string{"tag1": {"alias1"}, "tag2": {"alias2"}}, aliases)
		})
	}
}

type exportSceneReadError struct {
	models.SceneReaderWriter
	ids []int
}

func (s *exportSceneReadError) GetFiles(_ context.Context, id int) ([]*models.VideoFile, error) {
	s.ids = append(s.ids, id)
	return nil, errors.New("conn busy")
}

func TestExportSceneSkipsFailedRelationships(t *testing.T) {
	reader := &exportSceneReadError{}
	task := ExportTask{repository: models.Repository{Scene: reader}}
	jobs := make(chan *models.Scene, 2)
	for _, id := range []int{1, 2} {
		jobs <- &models.Scene{
			ID:           id,
			URLs:         models.NewRelatedStrings([]string{}),
			GalleryIDs:   models.NewRelatedIDs([]int{}),
			PerformerIDs: models.NewRelatedIDs([]int{}),
			TagIDs:       models.NewRelatedIDs([]int{}),
			Groups:       models.NewRelatedGroups([]models.GroupsScenes{}),
			StashIDs:     models.NewRelatedStashIDs([]models.StashID{}),
		}
	}
	close(jobs)
	var wg sync.WaitGroup
	wg.Add(1)
	// Call the worker directly so a regression is reported as a test failure.
	require.NotPanics(t, func() { task.exportScene(context.Background(), &wg, jobs) })
	wg.Wait()
	require.Equal(t, []int{1, 2}, reader.ids, "a failed item must not prevent reading the next item")
}

type exportAudioReadError struct {
	models.AudioReaderWriter
	ids []int
}

func (s *exportAudioReadError) GetFiles(_ context.Context, id int) ([]*models.AudioFile, error) {
	s.ids = append(s.ids, id)
	return nil, errors.New("conn busy")
}

func TestExportAudioSkipsFailedRelationships(t *testing.T) {
	reader := &exportAudioReadError{}
	task := ExportTask{repository: models.Repository{Audio: reader}}
	jobs := make(chan *models.Audio, 2)
	for _, id := range []int{1, 2} {
		jobs <- &models.Audio{
			ID:           id,
			URLs:         models.NewRelatedStrings([]string{}),
			GalleryIDs:   models.NewRelatedIDs([]int{}),
			PerformerIDs: models.NewRelatedIDs([]int{}),
			TagIDs:       models.NewRelatedIDs([]int{}),
			Groups:       models.NewRelatedGroupsAudio([]models.GroupsAudios{}),
		}
	}
	close(jobs)
	var wg sync.WaitGroup
	wg.Add(1)
	require.NotPanics(t, func() { task.exportAudio(context.Background(), &wg, jobs) })
	wg.Wait()
	require.Equal(t, []int{1, 2}, reader.ids, "a failed item must not prevent reading the next item")
}

type exportImageReader struct {
	models.ImageReaderWriter
}

func (*exportImageReader) GetCustomFields(context.Context, int) (map[string]interface{}, error) {
	return nil, nil
}

type exportGalleryReadError struct {
	models.GalleryReaderWriter
	ids []int
}

func (s *exportGalleryReadError) FindByImageID(_ context.Context, id int) ([]*models.Gallery, error) {
	s.ids = append(s.ids, id)
	return []*models.Gallery{{ID: id}}, nil
}

func (*exportGalleryReadError) GetFiles(context.Context, int) ([]models.File, error) {
	return nil, errors.New("conn busy")
}

func TestExportImageSkipsFailedGalleryFiles(t *testing.T) {
	reader := &exportGalleryReadError{}
	task := ExportTask{repository: models.Repository{
		Image:   &exportImageReader{},
		Gallery: reader,
	}}
	jobs := make(chan *models.Image, 2)
	for _, id := range []int{1, 2} {
		jobs <- &models.Image{
			ID:    id,
			URLs:  models.NewRelatedStrings([]string{}),
			Files: models.NewRelatedFiles([]models.File{}),
		}
	}
	close(jobs)
	var wg sync.WaitGroup
	wg.Add(1)
	require.NotPanics(t, func() { task.exportImage(context.Background(), &wg, jobs) })
	wg.Wait()
	require.Equal(t, []int{1, 2}, reader.ids, "a failed gallery read must skip the image and allow the next item")
}
