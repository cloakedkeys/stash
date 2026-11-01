package api

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/stashapp/stash/pkg/librarylimit"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
)

// The stores decide which objects an account with a library limit can see. The lists
// here are for everything else the API can do. They are lists of what is allowed, so
// anything not considered here, including anything added later, is refused.

// limitedQueries are the queries whose results the stores limit, or which don't
// involve the library. Questions about the library as a whole, like files, folders,
// duplicates and marker strings, aren't here.
var limitedQueries = map[string]bool{
	"findSavedFilter":       true,
	"findSavedFilters":      true,
	"findDefaultFilter":     true,
	"findScene":             true,
	"findSceneByHash":       true,
	"findScenes":            true,
	"findScenesByPathRegex": true,
	"sceneStreams":          true,
	"findSceneMarkers":      true,
	"sceneMarkerTags":       true,
	"findAudio":             true,
	"findAudios":            true,
	"findStory":             true,
	"findStories":           true,
	"findImage":             true,
	"findImages":            true,
	"findPerformer":         true,
	"findPerformers":        true,
	"findStudio":            true,
	"findStudios":           true,
	"findMovie":             true,
	"findMovies":            true,
	"findGroup":             true,
	"findGroups":            true,
	"findGallery":           true,
	"findGalleries":         true,
	"findTag":               true,
	"findTags":              true,
	"findVoiceActor":        true,
	"findVoiceActors":       true,
	"markerWall":            true,
	"sceneWall":             true,
	"stats":                 true,
	"plugins":               true,
	"listScrapers":          true,
	"configuration":         true,
	"systemStatus":          true,
	"getDatabaseBackend":    true,
	"version":               true,
	"latestversion":         true,
	"me":                    true,
}

// limitedAccountMutations only change the account's own settings.
var limitedAccountMutations = map[string]bool{
	"saveFilter":         true,
	"destroySavedFilter": true,
	"setDefaultFilter":   true,
	"configureUI":        true,
	"configureUISetting": true,
	"changePassword":     true,
	"generateAPIKey":     true,
}

// limitedObjectMutations change what belongs to the account on one object, which it must be able to see.
var limitedObjectMutations = map[string]librarylimit.Kind{
	"sceneIncrementO":          librarylimit.Scenes,
	"sceneDecrementO":          librarylimit.Scenes,
	"sceneAddO":                librarylimit.Scenes,
	"sceneDeleteO":             librarylimit.Scenes,
	"sceneResetO":              librarylimit.Scenes,
	"sceneSaveActivity":        librarylimit.Scenes,
	"sceneResetActivity":       librarylimit.Scenes,
	"sceneIncrementPlayCount":  librarylimit.Scenes,
	"sceneAddPlay":             librarylimit.Scenes,
	"sceneDeletePlay":          librarylimit.Scenes,
	"sceneResetPlayCount":      librarylimit.Scenes,
	"sceneUserDataUpdate":      librarylimit.Scenes,
	"audioAddO":                librarylimit.Audios,
	"audioDeleteO":             librarylimit.Audios,
	"audioResetO":              librarylimit.Audios,
	"audioSaveActivity":        librarylimit.Audios,
	"audioResetActivity":       librarylimit.Audios,
	"audioAddPlay":             librarylimit.Audios,
	"audioDeletePlay":          librarylimit.Audios,
	"audioResetPlayCount":      librarylimit.Audios,
	"audioUserDataUpdate":      librarylimit.Audios,
	"storyUserDataUpdate":      librarylimit.Stories,
	"imageIncrementO":          librarylimit.Images,
	"imageDecrementO":          librarylimit.Images,
	"imageResetO":              librarylimit.Images,
	"imageUserDataUpdate":      librarylimit.Images,
	"galleryIncrementO":        librarylimit.Galleries,
	"galleryDecrementO":        librarylimit.Galleries,
	"galleryResetO":            librarylimit.Galleries,
	"galleryUserDataUpdate":    librarylimit.Galleries,
	"groupUserDataUpdate":      librarylimit.Groups,
	"performerUserDataUpdate":  librarylimit.Performers,
	"studioUserDataUpdate":     librarylimit.Studios,
	"tagUserDataUpdate":        librarylimit.Tags,
	"voiceActorUserDataUpdate": librarylimit.VoiceActors,
}

// limitedFolderFields would list folders outside the limit.
var limitedFolderFields = map[string]bool{
	"sub_folders": true,
}

// A hidden object and one that doesn't exist get the same answer, so that an
// account can't use this to find out what exists.
var errLimitedNotFound = errors.New("not found")

// LibraryLimitMiddleware applies library limits to the API, other than what the stores decide.
func (r *Resolver) LibraryLimitMiddleware(ctx context.Context, next graphql.Resolver) (any, error) {
	if models.LibraryLimitFromContext(ctx) == nil {
		return next(ctx)
	}

	fc := graphql.GetFieldContext(ctx)
	if fc == nil || strings.HasPrefix(fc.Field.Name, "__") {
		return next(ctx)
	}

	name := fc.Field.Name

	switch fc.Object {
	case "Query":
		// the interface polls this. It isn't an error, there's just nothing to show.
		if name == "jobQueue" {
			return nil, nil
		}
		if !limitedQueries[name] {
			return nil, session.ErrUnauthorized
		}
	case "Mutation":
		if limitedAccountMutations[name] {
			return next(ctx)
		}
		kind, ok := limitedObjectMutations[name]
		if !ok {
			return nil, session.ErrUnauthorized
		}
		id, ok := limitedMutationObjectID(fc.Args)
		if !ok {
			return nil, session.ErrUnauthorized
		}
		visible, err := r.limitedObjectVisible(ctx, kind, id)
		if err != nil {
			return nil, err
		}
		if !visible {
			return nil, errLimitedNotFound
		}
	case "Subscription":
		return nil, session.ErrUnauthorized
	case "Folder":
		if limitedFolderFields[name] {
			return nil, session.ErrUnauthorized
		}
	}

	return next(ctx)
}

func limitedMutationObjectID(args map[string]any) (int, bool) {
	var s string
	if id, ok := args["id"].(string); ok {
		s = id
	} else {
		switch input := args["input"].(type) {
		case UserRatingFavoriteInput:
			s = input.ID
		case UserFavoriteInput:
			s = input.ID
		case UserRatingInput:
			s = input.ID
		default:
			return 0, false
		}
	}

	id, err := strconv.Atoi(s)
	return id, err == nil
}

func (r *Resolver) limitedObjectVisible(ctx context.Context, kind librarylimit.Kind, id int) (visible bool, err error) {
	err = r.withReadTxn(ctx, func(ctx context.Context) error {
		var err error
		switch kind {
		case librarylimit.Scenes:
			var o *models.Scene
			o, err = r.repository.Scene.Find(ctx, id)
			visible = o != nil
		case librarylimit.Audios:
			var o *models.Audio
			o, err = r.repository.Audio.Find(ctx, id)
			visible = o != nil
		case librarylimit.Stories:
			var o *models.Story
			o, err = r.repository.Story.Find(ctx, id)
			visible = o != nil
		case librarylimit.Images:
			var o *models.Image
			o, err = r.repository.Image.Find(ctx, id)
			visible = o != nil
		case librarylimit.Galleries:
			var o *models.Gallery
			o, err = r.repository.Gallery.Find(ctx, id)
			visible = o != nil
		case librarylimit.Groups:
			var o *models.Group
			o, err = r.repository.Group.Find(ctx, id)
			visible = o != nil
		case librarylimit.Performers:
			var o *models.Performer
			o, err = r.repository.Performer.Find(ctx, id)
			visible = o != nil
		case librarylimit.Studios:
			var o *models.Studio
			o, err = r.repository.Studio.Find(ctx, id)
			visible = o != nil
		case librarylimit.Tags:
			var o *models.Tag
			o, err = r.repository.Tag.Find(ctx, id)
			visible = o != nil
		case librarylimit.VoiceActors:
			var o *models.VoiceActor
			o, err = r.repository.VoiceActor.Find(ctx, id)
			visible = o != nil
		}
		return err
	})
	return visible, err
}

// withoutNil removes the objects that a library limit left out of a loader's results.
func withoutNil[T any](rows []*T) []*T {
	ret := make([]*T, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			ret = append(ret, row)
		}
	}

	return ret
}

func includesID(id int) *models.MultiCriterionInput {
	return &models.MultiCriterionInput{
		Value:    []string{strconv.Itoa(id)},
		Modifier: models.CriterionModifierIncludes,
	}
}

func includesIDHierarchical(id int) *models.HierarchicalMultiCriterionInput {
	return &models.HierarchicalMultiCriterionInput{
		Value:    []string{strconv.Itoa(id)},
		Modifier: models.CriterionModifierIncludes,
	}
}

// These counts are quick and direct in the stores, but those count everything. An
// account with a limit gets a count of what it can see.

func (r *Resolver) performerSceneCount(ctx context.Context, id int) (int, error) {
	if models.LibraryLimitFromContext(ctx) == nil {
		return r.repository.Scene.CountByPerformerID(ctx, id)
	}
	return r.repository.Scene.QueryCount(ctx, &models.SceneFilterType{Performers: includesID(id)}, nil)
}

func (r *Resolver) performerAudioCount(ctx context.Context, id int) (int, error) {
	if models.LibraryLimitFromContext(ctx) == nil {
		return r.repository.Audio.CountByPerformerID(ctx, id)
	}
	return r.repository.Audio.QueryCount(ctx, &models.AudioFilterType{Performers: includesID(id)}, nil)
}

func (r *Resolver) performerGroupCount(ctx context.Context, id int) (int, error) {
	if models.LibraryLimitFromContext(ctx) == nil {
		return r.repository.Group.CountByPerformerID(ctx, id)
	}
	return r.repository.Group.QueryCount(ctx, &models.GroupFilterType{Performers: includesID(id)}, nil)
}

func (r *Resolver) galleryImageCount(ctx context.Context, id int) (int, error) {
	if models.LibraryLimitFromContext(ctx) == nil {
		return r.repository.Image.CountByGalleryID(ctx, id)
	}
	return r.repository.Image.QueryCount(ctx, &models.ImageFilterType{Galleries: includesID(id)}, nil)
}

// The store's CountByParentTagID counts the parents of a tag, and CountByChildTagID its children.
func (r *Resolver) tagParentCount(ctx context.Context, id int) (int, error) {
	if models.LibraryLimitFromContext(ctx) == nil {
		return r.repository.Tag.CountByParentTagID(ctx, id)
	}
	return r.limitedTagCount(ctx, &models.TagFilterType{Children: includesIDHierarchical(id)})
}

func (r *Resolver) tagChildCount(ctx context.Context, id int) (int, error) {
	if models.LibraryLimitFromContext(ctx) == nil {
		return r.repository.Tag.CountByChildTagID(ctx, id)
	}
	return r.limitedTagCount(ctx, &models.TagFilterType{Parents: includesIDHierarchical(id)})
}

func (r *Resolver) limitedTagCount(ctx context.Context, filter *models.TagFilterType) (int, error) {
	one := 1
	_, count, err := r.repository.Tag.Query(ctx, filter, &models.FindFilterType{PerPage: &one})
	return count, err
}

// limitedStats returns the statistics of what the account can see. Totals of plays and
// O counts can't be worked out from the stores for part of the library, so they are left at zero.
func (r *queryResolver) limitedStats(ctx context.Context) (*StatsResultType, error) {
	var ret StatsResultType

	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		repo := r.repository

		one := 1
		page := &models.FindFilterType{PerPage: &one}

		scenes, err := repo.Scene.Query(ctx, models.SceneQueryOptions{
			QueryOptions:  models.QueryOptions{FindFilter: page, Count: true},
			TotalDuration: true,
			TotalSize:     true,
		})
		if err != nil {
			return err
		}
		ret.SceneCount = scenes.Count
		ret.ScenesSize = scenes.TotalSize
		ret.ScenesDuration = scenes.TotalDuration

		audios, err := repo.Audio.Query(ctx, models.AudioQueryOptions{
			QueryOptions:  models.QueryOptions{FindFilter: page, Count: true},
			TotalDuration: true,
			TotalSize:     true,
		})
		if err != nil {
			return err
		}
		ret.AudioCount = audios.Count
		ret.AudiosSize = audios.TotalSize
		ret.AudiosDuration = audios.TotalDuration

		images, err := repo.Image.Query(ctx, models.ImageQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: page, Count: true},
			TotalSize:    true,
		})
		if err != nil {
			return err
		}
		ret.ImageCount = images.Count
		ret.ImagesSize = images.TotalSize

		if ret.GalleryCount, err = repo.Gallery.QueryCount(ctx, nil, nil); err != nil {
			return err
		}
		if ret.PerformerCount, err = repo.Performer.QueryCount(ctx, nil, nil); err != nil {
			return err
		}
		if ret.StudioCount, err = repo.Studio.QueryCount(ctx, nil, nil); err != nil {
			return err
		}
		if ret.GroupCount, err = repo.Group.QueryCount(ctx, nil, nil); err != nil {
			return err
		}
		ret.MovieCount = ret.GroupCount
		if ret.TagCount, err = r.limitedTagCount(ctx, nil); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &ret, nil
}
