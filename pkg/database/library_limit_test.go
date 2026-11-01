//go:build integration

package database_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sliceutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// limitFixture is a small library with everything a limit can allow or hide. The
// shared objects of the "hidden" content are only linked to hidden content.
type limitFixture struct {
	rootFolder, subFolder, hiddenFolder models.FolderID

	tagAllowed, tagAllowedChild, tagShared, tagMarker, tagHidden, tagPerformerShared int
	studioAllowed, studioAllowedChild, studioShared, studioHidden                    int
	tagStoryVisible, tagStoryHidden                                                  int

	sceneInFolder, sceneTagged, sceneInStudio, sceneHidden int
	audioInFolder, audioHidden                             int
	storyInFolder, storyTagged, storyHidden                int
	storyFiles                                             map[int]models.FileID
	imageInFolder, imageInGallery, imageHidden             int
	galleryOfFiles, galleryOfFolder, galleryHidden         int
	markerVisible, markerHidden                            int
	performerVisible, performerHidden                      int
	groupVisible, groupHidden                              int
	actorVisible, actorHidden                              int
}

func createLimitFixture(t *testing.T, ctx context.Context) *limitFixture {
	t.Helper()
	f := &limitFixture{}
	n := 0

	folder := func(path string, parent *models.FolderID) models.FolderID {
		o := &models.Folder{Path: path, ParentFolderID: parent}
		require.NoError(t, db.Folder().Create(ctx, o))
		return o.ID
	}
	base := func(parent models.FolderID) *models.BaseFile {
		n++
		return &models.BaseFile{Basename: fmt.Sprintf("limit_%d", n), ParentFolderID: parent}
	}
	videoFile := func(parent models.FolderID) models.FileID {
		o := &models.VideoFile{BaseFile: base(parent), Format: "mp4", Width: 1, Height: 1, Duration: 1}
		require.NoError(t, db.File().Create(ctx, o))
		return o.ID
	}
	audioFile := func(parent models.FolderID) models.FileID {
		o := &models.AudioFile{BaseFile: base(parent), Format: "mp3", Duration: 1}
		require.NoError(t, db.File().Create(ctx, o))
		return o.ID
	}
	imageFile := func(parent models.FolderID) models.FileID {
		o := &models.ImageFile{BaseFile: base(parent), Format: "jpg", Width: 1, Height: 1}
		require.NoError(t, db.File().Create(ctx, o))
		return o.ID
	}
	zipFile := func(parent models.FolderID) models.FileID {
		o := base(parent)
		require.NoError(t, db.File().Create(ctx, o))
		return o.ID
	}
	tag := func(name string, parent ...int) int {
		o := &models.Tag{Name: "limit " + name}
		require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: o}))
		if len(parent) > 0 {
			require.NoError(t, db.Tag().UpdateParentTags(ctx, o.ID, parent))
		}
		return o.ID
	}
	studio := func(name string, parent *int) int {
		o := &models.Studio{Name: "limit " + name, ParentID: parent}
		require.NoError(t, db.Studio().Create(ctx, &models.CreateStudioInput{Studio: o}))
		return o.ID
	}
	performer := func(name string, tags ...int) int {
		o := &models.Performer{Name: "limit " + name, TagIDs: models.NewRelatedIDs(tags)}
		require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: o}))
		return o.ID
	}
	group := func(name string) int {
		o := &models.Group{Name: "limit " + name}
		require.NoError(t, db.Group().Create(ctx, o))
		return o.ID
	}
	actor := func(name string) int {
		o := &models.VoiceActor{Name: "limit " + name}
		require.NoError(t, db.VoiceActor().Create(ctx, o))
		return o.ID
	}
	scene := func(parent models.FolderID, studioID *int, tags, performers []int, groupID *int) int {
		o := &models.Scene{
			Title:        fmt.Sprintf("limit scene %d", n),
			Details:      "limit",
			StudioID:     studioID,
			TagIDs:       models.NewRelatedIDs(tags),
			PerformerIDs: models.NewRelatedIDs(performers),
		}
		if groupID != nil {
			o.Groups = models.NewRelatedGroups([]models.GroupsScenes{{GroupID: *groupID}})
		}
		require.NoError(t, db.Scene().Create(ctx, o, []models.FileID{videoFile(parent)}))
		return o.ID
	}
	audio := func(parent models.FolderID, tags, performers []int) int {
		o := &models.Audio{
			Title:        fmt.Sprintf("limit audio %d", n),
			TagIDs:       models.NewRelatedIDs(tags),
			PerformerIDs: models.NewRelatedIDs(performers),
		}
		require.NoError(t, db.Audio().Create(ctx, o, []models.FileID{audioFile(parent)}))
		return o.ID
	}
	f.storyFiles = make(map[int]models.FileID)
	story := func(parent models.FolderID, tags []int) int {
		file := base(parent)
		require.NoError(t, db.File().Create(ctx, file))
		o := models.NewStory()
		o.Title = fmt.Sprintf("limit story %d", n)
		o.TagIDs = models.NewRelatedIDs(tags)
		require.NoError(t, db.Story().Create(ctx, &o, []models.FileID{file.ID}))
		f.storyFiles[o.ID] = file.ID
		return o.ID
	}
	image := func(parent models.FolderID, tags []int) int {
		o := &models.Image{Title: fmt.Sprintf("limit image %d", n), TagIDs: models.NewRelatedIDs(tags)}
		require.NoError(t, db.Image().Create(ctx, &models.CreateImageInput{Image: o, FileIDs: []models.FileID{imageFile(parent)}}))
		return o.ID
	}
	marker := func(sceneID, primaryTag int) int {
		o := &models.SceneMarker{Title: "limit marker", SceneID: sceneID, PrimaryTagID: primaryTag}
		require.NoError(t, db.SceneMarker().Create(ctx, o))
		return o.ID
	}

	f.rootFolder = folder("/limit-test/allowed", nil)
	f.subFolder = folder("/limit-test/allowed/sub", &f.rootFolder)
	f.hiddenFolder = folder("/limit-test/hidden", nil)

	f.tagAllowed = tag("allowed")
	f.tagAllowedChild = tag("allowed child", f.tagAllowed)
	f.tagShared = tag("shared")
	f.tagMarker = tag("marker")
	f.tagHidden = tag("hidden")
	f.tagPerformerShared = tag("performer shared")
	f.tagStoryVisible = tag("story visible")
	f.tagStoryHidden = tag("story hidden")

	f.studioAllowed = studio("allowed", nil)
	f.studioAllowedChild = studio("allowed child", &f.studioAllowed)
	f.studioShared = studio("shared", nil)
	f.studioHidden = studio("hidden", nil)

	f.performerVisible = performer("visible", f.tagPerformerShared)
	f.performerHidden = performer("hidden", f.tagHidden)
	f.groupVisible = group("visible")
	f.groupHidden = group("hidden")
	f.actorVisible = actor("visible")
	f.actorHidden = actor("hidden")

	f.sceneInFolder = scene(f.subFolder, &f.studioShared, []int{f.tagShared}, []int{f.performerVisible}, &f.groupVisible)
	f.sceneTagged = scene(f.hiddenFolder, nil, []int{f.tagAllowedChild}, nil, nil)
	f.sceneInStudio = scene(f.hiddenFolder, &f.studioAllowedChild, nil, nil, nil)
	f.sceneHidden = scene(f.hiddenFolder, &f.studioHidden, []int{f.tagHidden}, []int{f.performerHidden}, &f.groupHidden)

	require.NoError(t, db.VoiceActor().SetScene(ctx, f.sceneInFolder, []int{f.actorVisible}))
	require.NoError(t, db.VoiceActor().SetScene(ctx, f.sceneHidden, []int{f.actorHidden}))

	f.markerVisible = marker(f.sceneInFolder, f.tagMarker)
	f.markerHidden = marker(f.sceneHidden, f.tagHidden)
	require.NoError(t, db.SceneMarker().UpdateTags(ctx, f.markerVisible, []int{f.tagMarker}))
	require.NoError(t, db.SceneMarker().UpdateTags(ctx, f.markerHidden, []int{f.tagHidden}))

	f.audioInFolder = audio(f.subFolder, nil, nil)
	f.audioHidden = audio(f.hiddenFolder, []int{f.tagHidden}, []int{f.performerHidden})
	f.storyInFolder = story(f.subFolder, []int{f.tagStoryVisible})
	f.storyTagged = story(f.hiddenFolder, []int{f.tagAllowedChild, f.tagStoryVisible})
	f.storyHidden = story(f.hiddenFolder, []int{f.tagStoryHidden})

	f.imageInFolder = image(f.subFolder, nil)
	f.imageInGallery = image(f.hiddenFolder, nil)
	f.imageHidden = image(f.hiddenFolder, []int{f.tagHidden})

	galleryFile := &models.Gallery{Title: "limit gallery of files"}
	require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: galleryFile, FileIDs: []models.FileID{zipFile(f.subFolder)}}))
	f.galleryOfFiles = galleryFile.ID
	require.NoError(t, db.Gallery().AddImages(ctx, f.galleryOfFiles, f.imageInGallery))

	galleryFolder := &models.Gallery{Title: "limit gallery of folder", FolderID: &f.subFolder}
	require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: galleryFolder}))
	f.galleryOfFolder = galleryFolder.ID

	galleryHidden := &models.Gallery{Title: "limit gallery hidden", FolderID: &f.hiddenFolder}
	require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: galleryHidden}))
	f.galleryHidden = galleryHidden.ID
	require.NoError(t, db.Gallery().AddImages(ctx, f.galleryHidden, f.imageHidden))

	return f
}

// limitReads are the ways of reading one kind of object from its store.
type limitReads struct {
	find  func(ctx context.Context, id int) (bool, error)
	many  func(ctx context.Context, ids []int) ([]int, error)
	query func(ctx context.Context) ([]int, error)
	count func(ctx context.Context) (int, error)
	all   func(ctx context.Context) ([]int, error)
}

func ids[T any](rows []*T, err error, id func(*T) int) ([]int, error) {
	if err != nil {
		return nil, err
	}
	return sliceutil.Map(rows, id), nil
}

func perPageAll() *models.FindFilterType {
	all := -1
	return &models.FindFilterType{PerPage: &all}
}

func limitReadsOfKinds() map[string]limitReads {
	return map[string]limitReads{
		"scenes": {
			find: func(ctx context.Context, id int) (bool, error) {
				o, err := db.Scene().Find(ctx, id)
				return o != nil, err
			},
			many: func(ctx context.Context, in []int) ([]int, error) {
				rows, err := db.Scene().FindByIDs(ctx, in)
				return ids(rows, err, func(o *models.Scene) int { return o.ID })
			},
			query: func(ctx context.Context) ([]int, error) {
				r, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll(), Count: true}})
				if err != nil {
					return nil, err
				}
				return r.IDs, nil
			},
			count: func(ctx context.Context) (int, error) { return db.Scene().QueryCount(ctx, nil, nil) },
			all: func(ctx context.Context) ([]int, error) {
				rows, err := db.Scene().All(ctx)
				return ids(rows, err, func(o *models.Scene) int { return o.ID })
			},
		},
		"audios": {
			find: func(ctx context.Context, id int) (bool, error) {
				o, err := db.Audio().Find(ctx, id)
				return o != nil, err
			},
			many: func(ctx context.Context, in []int) ([]int, error) {
				rows, err := db.Audio().FindByIDs(ctx, in)
				return ids(rows, err, func(o *models.Audio) int { return o.ID })
			},
			query: func(ctx context.Context) ([]int, error) {
				r, err := db.Audio().Query(ctx, models.AudioQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll(), Count: true}})
				if err != nil {
					return nil, err
				}
				return r.IDs, nil
			},
			count: func(ctx context.Context) (int, error) { return db.Audio().QueryCount(ctx, nil, nil) },
			all: func(ctx context.Context) ([]int, error) {
				rows, err := db.Audio().All(ctx)
				return ids(rows, err, func(o *models.Audio) int { return o.ID })
			},
		},
		"stories": {
			find: func(ctx context.Context, id int) (bool, error) {
				o, err := db.Story().Find(ctx, id)
				return o != nil, err
			},
			many: func(ctx context.Context, in []int) ([]int, error) {
				rows, err := db.Story().FindMany(ctx, in)
				return ids(rows, err, func(o *models.Story) int { return o.ID })
			},
			query: func(ctx context.Context) ([]int, error) {
				r, err := db.Story().Query(ctx, models.StoryQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll(), Count: true}})
				if err != nil {
					return nil, err
				}
				return r.IDs, nil
			},
			count: func(ctx context.Context) (int, error) { return db.Story().QueryCount(ctx, nil, nil) },
		},
		"images": {
			find: func(ctx context.Context, id int) (bool, error) {
				o, err := db.Image().Find(ctx, id)
				return o != nil, err
			},
			many: func(ctx context.Context, in []int) ([]int, error) {
				rows, err := db.Image().FindMany(ctx, in)
				return ids(rows, err, func(o *models.Image) int { return o.ID })
			},
			query: func(ctx context.Context) ([]int, error) {
				r, err := db.Image().Query(ctx, models.ImageQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll(), Count: true}})
				if err != nil {
					return nil, err
				}
				return r.IDs, nil
			},
			count: func(ctx context.Context) (int, error) { return db.Image().QueryCount(ctx, nil, nil) },
			all: func(ctx context.Context) ([]int, error) {
				rows, err := db.Image().All(ctx)
				return ids(rows, err, func(o *models.Image) int { return o.ID })
			},
		},
		"galleries": {
			find: func(ctx context.Context, id int) (bool, error) {
				o, err := db.Gallery().Find(ctx, id)
				return o != nil, err
			},
			many: func(ctx context.Context, in []int) ([]int, error) {
				rows, err := db.Gallery().FindMany(ctx, in)
				return ids(rows, err, func(o *models.Gallery) int { return o.ID })
			},
			query: func(ctx context.Context) ([]int, error) {
				rows, _, err := db.Gallery().Query(ctx, nil, perPageAll())
				return ids(rows, err, func(o *models.Gallery) int { return o.ID })
			},
			count: func(ctx context.Context) (int, error) { return db.Gallery().QueryCount(ctx, nil, nil) },
			all: func(ctx context.Context) ([]int, error) {
				rows, err := db.Gallery().All(ctx)
				return ids(rows, err, func(o *models.Gallery) int { return o.ID })
			},
		},
		"markers": {
			find: func(ctx context.Context, id int) (bool, error) {
				o, err := db.SceneMarker().Find(ctx, id)
				return o != nil, err
			},
			many: func(ctx context.Context, in []int) ([]int, error) {
				rows, err := db.SceneMarker().FindMany(ctx, in)
				return ids(rows, err, func(o *models.SceneMarker) int { return o.ID })
			},
			query: func(ctx context.Context) ([]int, error) {
				rows, _, err := db.SceneMarker().Query(ctx, nil, perPageAll())
				return ids(rows, err, func(o *models.SceneMarker) int { return o.ID })
			},
			count: func(ctx context.Context) (int, error) { return db.SceneMarker().QueryCount(ctx, nil, nil) },
			all: func(ctx context.Context) ([]int, error) {
				rows, err := db.SceneMarker().All(ctx)
				return ids(rows, err, func(o *models.SceneMarker) int { return o.ID })
			},
		},
		"performers": {
			find: func(ctx context.Context, id int) (bool, error) {
				o, err := db.Performer().Find(ctx, id)
				return o != nil, err
			},
			many: func(ctx context.Context, in []int) ([]int, error) {
				rows, err := db.Performer().FindMany(ctx, in)
				return ids(rows, err, func(o *models.Performer) int { return o.ID })
			},
			query: func(ctx context.Context) ([]int, error) {
				rows, _, err := db.Performer().Query(ctx, nil, perPageAll())
				return ids(rows, err, func(o *models.Performer) int { return o.ID })
			},
			count: func(ctx context.Context) (int, error) { return db.Performer().QueryCount(ctx, nil, nil) },
			all: func(ctx context.Context) ([]int, error) {
				rows, err := db.Performer().All(ctx)
				return ids(rows, err, func(o *models.Performer) int { return o.ID })
			},
		},
		"tags": {
			find: func(ctx context.Context, id int) (bool, error) {
				o, err := db.Tag().Find(ctx, id)
				return o != nil, err
			},
			many: func(ctx context.Context, in []int) ([]int, error) {
				rows, err := db.Tag().FindMany(ctx, in)
				return ids(rows, err, func(o *models.Tag) int { return o.ID })
			},
			query: func(ctx context.Context) ([]int, error) {
				rows, _, err := db.Tag().Query(ctx, nil, perPageAll())
				return ids(rows, err, func(o *models.Tag) int { return o.ID })
			},
			count: func(ctx context.Context) (int, error) {
				_, count, err := db.Tag().Query(ctx, nil, perPageAll())
				return count, err
			},
			all: func(ctx context.Context) ([]int, error) {
				rows, err := db.Tag().All(ctx)
				return ids(rows, err, func(o *models.Tag) int { return o.ID })
			},
		},
		"studios": {
			find: func(ctx context.Context, id int) (bool, error) {
				o, err := db.Studio().Find(ctx, id)
				return o != nil, err
			},
			many: func(ctx context.Context, in []int) ([]int, error) {
				rows, err := db.Studio().FindMany(ctx, in)
				return ids(rows, err, func(o *models.Studio) int { return o.ID })
			},
			query: func(ctx context.Context) ([]int, error) {
				rows, _, err := db.Studio().Query(ctx, nil, perPageAll())
				return ids(rows, err, func(o *models.Studio) int { return o.ID })
			},
			count: func(ctx context.Context) (int, error) { return db.Studio().QueryCount(ctx, nil, nil) },
			all: func(ctx context.Context) ([]int, error) {
				rows, err := db.Studio().All(ctx)
				return ids(rows, err, func(o *models.Studio) int { return o.ID })
			},
		},
		"groups": {
			find: func(ctx context.Context, id int) (bool, error) {
				o, err := db.Group().Find(ctx, id)
				return o != nil, err
			},
			many: func(ctx context.Context, in []int) ([]int, error) {
				rows, err := db.Group().FindMany(ctx, in)
				return ids(rows, err, func(o *models.Group) int { return o.ID })
			},
			query: func(ctx context.Context) ([]int, error) {
				rows, _, err := db.Group().Query(ctx, nil, perPageAll())
				return ids(rows, err, func(o *models.Group) int { return o.ID })
			},
			count: func(ctx context.Context) (int, error) { return db.Group().QueryCount(ctx, nil, nil) },
			all: func(ctx context.Context) ([]int, error) {
				rows, err := db.Group().All(ctx)
				return ids(rows, err, func(o *models.Group) int { return o.ID })
			},
		},
		"voice actors": {
			find: func(ctx context.Context, id int) (bool, error) {
				o, err := db.VoiceActor().Find(ctx, id)
				return o != nil, err
			},
			query: func(ctx context.Context) ([]int, error) {
				rows, _, err := db.VoiceActor().Query(ctx, perPageAll(), nil)
				return ids(rows, err, func(o *models.VoiceActor) int { return o.ID })
			},
			count: func(ctx context.Context) (int, error) {
				_, count, err := db.VoiceActor().Query(ctx, perPageAll(), nil)
				return count, err
			},
		},
	}
}

func (f *limitFixture) allObjects() map[string][]int {
	return map[string][]int{
		"scenes":       {f.sceneInFolder, f.sceneTagged, f.sceneInStudio, f.sceneHidden},
		"audios":       {f.audioInFolder, f.audioHidden},
		"stories":      {f.storyInFolder, f.storyTagged, f.storyHidden},
		"images":       {f.imageInFolder, f.imageInGallery, f.imageHidden},
		"galleries":    {f.galleryOfFiles, f.galleryOfFolder, f.galleryHidden},
		"markers":      {f.markerVisible, f.markerHidden},
		"performers":   {f.performerVisible, f.performerHidden},
		"tags":         {f.tagAllowed, f.tagAllowedChild, f.tagShared, f.tagMarker, f.tagHidden, f.tagPerformerShared, f.tagStoryVisible, f.tagStoryHidden},
		"studios":      {f.studioAllowed, f.studioAllowedChild, f.studioShared, f.studioHidden},
		"groups":       {f.groupVisible, f.groupHidden},
		"voice actors": {f.actorVisible, f.actorHidden},
	}
}

func TestLibraryLimitReads(t *testing.T) {
	runWithRollbackTxn(t, "matrix", func(t *testing.T, unlimited context.Context) {
		f := createLimitFixture(t, unlimited)
		objects := f.allObjects()
		reads := limitReadsOfKinds()

		tests := []struct {
			name  string
			limit *models.LibraryLimit
			want  map[string][]int
		}{
			{
				name:  "folder and its sub-folders",
				limit: &models.LibraryLimit{FolderIDs: []int{int(f.rootFolder)}},
				want: map[string][]int{
					"scenes":       {f.sceneInFolder},
					"audios":       {f.audioInFolder},
					"stories":      {f.storyInFolder},
					"images":       {f.imageInFolder, f.imageInGallery},
					"galleries":    {f.galleryOfFiles, f.galleryOfFolder},
					"markers":      {f.markerVisible},
					"performers":   {f.performerVisible},
					"tags":         {f.tagShared, f.tagMarker, f.tagPerformerShared, f.tagStoryVisible},
					"studios":      {f.studioShared},
					"groups":       {f.groupVisible},
					"voice actors": {f.actorVisible},
				},
			},
			{
				name:  "tag and its sub-tags",
				limit: &models.LibraryLimit{TagIDs: []int{f.tagAllowed}},
				want: map[string][]int{
					"scenes":  {f.sceneTagged},
					"stories": {f.storyTagged},
					"tags":    {f.tagAllowed, f.tagAllowedChild, f.tagStoryVisible},
				},
			},
			{
				name:  "studio and its sub-studios",
				limit: &models.LibraryLimit{StudioIDs: []int{f.studioAllowed}},
				want: map[string][]int{
					"scenes":  {f.sceneInStudio},
					"studios": {f.studioAllowed, f.studioAllowedChild},
				},
			},
			{
				name:  "any of several",
				limit: &models.LibraryLimit{TagIDs: []int{f.tagAllowed}, StudioIDs: []int{f.studioAllowed}},
				want: map[string][]int{
					"scenes":  {f.sceneTagged, f.sceneInStudio},
					"stories": {f.storyTagged},
					"tags":    {f.tagAllowed, f.tagAllowedChild, f.tagStoryVisible},
					"studios": {f.studioAllowed, f.studioAllowedChild},
				},
			},
			{
				name:  "a limit that lists nothing allows nothing",
				limit: &models.LibraryLimit{},
				want:  map[string][]int{},
			},
			{
				name:  "IDs that no longer exist allow nothing",
				limit: &models.LibraryLimit{FolderIDs: []int{-1}, TagIDs: []int{-1}, StudioIDs: []int{-1}},
				want:  map[string][]int{},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ctx := models.WithLibraryLimit(unlimited, tt.limit)
				for storyID, fileID := range f.storyFiles {
					got, err := db.Story().FindByFileID(ctx, fileID)
					require.NoError(t, err)
					var want []int
					if slices.Contains(tt.want["stories"], storyID) {
						want = []int{storyID}
					}
					assert.ElementsMatch(t, want, sliceutil.Map(got, func(o *models.Story) int { return o.ID }), "stories: file %d", fileID)
				}
				for kind, r := range reads {
					want := slices.Sorted(slices.Values(tt.want[kind]))

					for _, id := range objects[kind] {
						found, err := r.find(ctx, id)
						require.NoError(t, err, kind)
						assert.Equal(t, slices.Contains(want, id), found, "%s: find %d", kind, id)
					}

					if r.many != nil {
						got, err := r.many(ctx, objects[kind])
						require.NoError(t, err, kind)
						assert.ElementsMatch(t, want, got, "%s: many", kind)
					}

					got, err := r.query(ctx)
					require.NoError(t, err, kind)
					assert.ElementsMatch(t, want, got, "%s: query", kind)

					count, err := r.count(ctx)
					require.NoError(t, err, kind)
					assert.Equal(t, len(want), count, "%s: count", kind)

					if r.all != nil {
						got, err := r.all(ctx)
						require.NoError(t, err, kind)
						assert.ElementsMatch(t, want, got, "%s: all", kind)
					}
				}
			})
		}

		t.Run("no limit sees everything", func(t *testing.T) {
			for kind, r := range reads {
				got, err := r.query(unlimited)
				require.NoError(t, err, kind)
				assert.Subset(t, got, objects[kind], kind)
			}
		})
	})
}

// The stores have other ways of finding objects, some of them written in SQL.
func TestLibraryLimitOtherFinders(t *testing.T) {
	runWithRollbackTxn(t, "finders", func(t *testing.T, unlimited context.Context) {
		f := createLimitFixture(t, unlimited)
		ctx := models.WithLibraryLimit(unlimited, &models.LibraryLimit{FolderIDs: []int{int(f.rootFolder)}})

		tagIDs := func(rows []*models.Tag, err error) []int {
			require.NoError(t, err)
			return sliceutil.Map(rows, func(o *models.Tag) int { return o.ID })
		}
		sceneIDs := func(rows []*models.Scene, err error) []int {
			require.NoError(t, err)
			return sliceutil.Map(rows, func(o *models.Scene) int { return o.ID })
		}
		markerIDs := func(rows []*models.SceneMarker, err error) []int {
			require.NoError(t, err)
			return sliceutil.Map(rows, func(o *models.SceneMarker) int { return o.ID })
		}
		performerIDs := func(rows []*models.Performer, err error) []int {
			require.NoError(t, err)
			return sliceutil.Map(rows, func(o *models.Performer) int { return o.ID })
		}
		imageIDs := func(rows []*models.Image, err error) []int {
			require.NoError(t, err)
			return sliceutil.Map(rows, func(o *models.Image) int { return o.ID })
		}
		groupIDs := func(rows []*models.Group, err error) []int {
			require.NoError(t, err)
			return sliceutil.Map(rows, func(o *models.Group) int { return o.ID })
		}

		assert.Empty(t, tagIDs(db.Tag().FindBySceneID(ctx, f.sceneHidden)))
		assert.Equal(t, []int{f.tagShared}, tagIDs(db.Tag().FindBySceneID(ctx, f.sceneInFolder)))
		assert.Empty(t, tagIDs(db.Tag().FindByPerformerID(ctx, f.performerHidden)))
		assert.Empty(t, tagIDs(db.Tag().FindByNames(ctx, []string{"limit hidden"}, true)))
		assert.Equal(t, []int{f.tagMarker}, tagIDs(db.Tag().FindByNames(ctx, []string{"limit marker"}, true)))
		assert.Equal(t, []int{f.tagMarker}, tagIDs(db.Tag().FindBySceneMarkerID(ctx, f.markerVisible)))
		assert.Empty(t, tagIDs(db.Tag().FindBySceneMarkerID(ctx, f.markerHidden)))

		hiddenTag, err := db.Tag().FindByName(ctx, "limit hidden", true)
		require.NoError(t, err)
		assert.Nil(t, hiddenTag)

		assert.Empty(t, sceneIDs(db.Scene().FindByPerformerID(ctx, f.performerHidden)))
		assert.Equal(t, []int{f.sceneInFolder}, sceneIDs(db.Scene().FindByPerformerID(ctx, f.performerVisible)))
		assert.Empty(t, sceneIDs(db.Scene().FindByGroupID(ctx, f.groupHidden)))
		assert.Equal(t, []int{f.sceneInFolder}, sceneIDs(db.Scene().Wall(ctx, nil)))

		// a lookup by ID keeps working around what's hidden, instead of failing
		many, err := db.Scene().FindMany(ctx, []int{f.sceneHidden, f.sceneInFolder})
		require.NoError(t, err)
		require.Len(t, many, 1)
		assert.Equal(t, f.sceneInFolder, many[0].ID)

		assert.Equal(t, []int{f.markerVisible}, markerIDs(db.SceneMarker().FindBySceneID(ctx, f.sceneInFolder)))
		assert.Empty(t, markerIDs(db.SceneMarker().FindBySceneID(ctx, f.sceneHidden)))
		assert.Equal(t, []int{f.markerVisible}, markerIDs(db.SceneMarker().Wall(ctx, nil)))

		assert.Empty(t, performerIDs(db.Performer().FindBySceneID(ctx, f.sceneHidden)))
		assert.Empty(t, performerIDs(db.Performer().FindByNames(ctx, []string{"limit hidden"}, true)))

		assert.Equal(t, []int{f.imageInGallery}, imageIDs(db.Image().FindByGalleryID(ctx, f.galleryOfFiles)))
		assert.Empty(t, imageIDs(db.Image().FindByGalleryID(ctx, f.galleryHidden)))

		assert.Empty(t, groupIDs(db.Group().FindByNames(ctx, []string{"limit hidden"}, true)))
		assert.Empty(t, groupIDs(db.Group().FindByStudioID(ctx, f.studioHidden)))

		hiddenStudio, err := db.Studio().FindByName(ctx, "limit hidden", true)
		require.NoError(t, err)
		assert.Nil(t, hiddenStudio)

		hiddenGroup, err := db.Group().FindByName(ctx, "limit hidden", true)
		require.NoError(t, err)
		assert.Nil(t, hiddenGroup)

		hiddenActor, err := db.VoiceActor().FindByName(ctx, "limit hidden")
		require.NoError(t, err)
		assert.Nil(t, hiddenActor)
	})
}

func TestUserLibraryLimitStored(t *testing.T) {
	runWithRollbackTxn(t, "user", func(t *testing.T, ctx context.Context) {
		qb := db.User()
		limit := &models.LibraryLimit{FolderIDs: []int{1}, TagIDs: []int{2, 3}, StudioIDs: []int{}}

		u := &models.User{Username: "limit-created", Roles: models.Roles{models.RoleEnumRead}, LibraryLimit: limit, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		require.NoError(t, qb.Create(ctx, u, ""))

		got, err := qb.FindByUsername(ctx, u.Username)
		require.NoError(t, err)
		assert.Equal(t, limit, got.LibraryLimit)

		// editing an account's name or notes leaves its limit alone
		got.Notes = "edited"
		got.LibraryLimit = nil
		require.NoError(t, qb.Update(ctx, got))
		got, err = qb.FindByUsername(ctx, u.Username)
		require.NoError(t, err)
		assert.Equal(t, limit, got.LibraryLimit)

		all, err := qb.All(ctx)
		require.NoError(t, err)
		listed := slices.IndexFunc(all, func(o *models.User) bool { return o.ID == u.ID })
		require.GreaterOrEqual(t, listed, 0)
		assert.Equal(t, limit, all[listed].LibraryLimit)

		require.NoError(t, qb.SetLibraryLimit(ctx, u.ID, &models.LibraryLimit{}))
		got, err = qb.FindByUsername(ctx, u.Username)
		require.NoError(t, err)
		require.NotNil(t, got.LibraryLimit)
		assert.Empty(t, got.LibraryLimit.FolderIDs)

		// a limit that can't be read allows nothing, rather than everything
		_, _, err = db.ExecSQL(ctx, fmt.Sprintf(`UPDATE "users" SET "library_limit" = 'not json' WHERE "id" = %d`, u.ID), nil)
		require.NoError(t, err)
		got, err = qb.FindByUsername(ctx, u.Username)
		require.NoError(t, err)
		require.NotNil(t, got.LibraryLimit)
		assert.Empty(t, got.LibraryLimit.FolderIDs)
		assert.Empty(t, got.LibraryLimit.TagIDs)
		assert.Empty(t, got.LibraryLimit.StudioIDs)

		require.NoError(t, qb.SetLibraryLimit(ctx, u.ID, nil))
		got, err = qb.FindByUsername(ctx, u.Username)
		require.NoError(t, err)
		assert.Nil(t, got.LibraryLimit)

		other := &models.User{Username: "limit-none", Roles: models.Roles{models.RoleEnumRead}, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		require.NoError(t, qb.Create(ctx, other, ""))
		got, err = qb.FindByUsername(ctx, other.Username)
		require.NoError(t, err)
		assert.Nil(t, got.LibraryLimit)
	})
}
