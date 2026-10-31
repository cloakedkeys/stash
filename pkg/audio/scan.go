package audio

import (
	"context"
	"errors"
	"fmt"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/plugin"
	"github.com/stashapp/stash/pkg/plugin/hook"
)

var (
	ErrNotAudioFile = errors.New("not a audio file")

	// fingerprint types to match with
	// only try to match by data fingerprints, _not_ perceptual fingerprints
	matchableFingerprintTypes = []string{models.FingerprintTypeMD5}
)

type ScanCreatorUpdater interface {
	FindByFileID(ctx context.Context, fileID models.FileID) ([]*models.Audio, error)
	FindByFingerprints(ctx context.Context, fp []models.Fingerprint) ([]*models.Audio, error)
	GetFiles(ctx context.Context, relatedID int) ([]*models.AudioFile, error)

	Create(ctx context.Context, newAudio *models.Audio, fileIDs []models.FileID) error
	UpdatePartial(ctx context.Context, id int, updatedAudio models.AudioPartial) (*models.Audio, error)
	AddFileID(ctx context.Context, id int, fileID models.FileID) error
	UpdateCover(ctx context.Context, audioID int, cover []byte) error
}

// ScanNameFinder finds the existing objects that the metadata embedded in an
// audio file refers to. Objects are never created from embedded metadata.
type ScanNameFinder struct {
	Performer interface {
		FindByNames(ctx context.Context, names []string, nocase bool) ([]*models.Performer, error)
	}
	Tag interface {
		FindByNames(ctx context.Context, names []string, nocase bool) ([]*models.Tag, error)
	}
	Group interface {
		FindByName(ctx context.Context, name string, nocase bool) (*models.Group, error)
	}
}

type ScanGalleryFinderUpdater interface {
	FindByPath(ctx context.Context, p string) ([]*models.Gallery, error)
	AddAudioIDs(ctx context.Context, galleryID int, audioIDs []int) error
}

type ScanGenerator interface {
	Generate(ctx context.Context, s *models.Audio, f *models.AudioFile) error
}

type ScanHandler struct {
	CreatorUpdater ScanCreatorUpdater

	// optional - if set, new audios are populated from the file's metadata
	MetadataReader MetadataReader
	NameFinder     ScanNameFinder

	PluginCache *plugin.Cache
	Paths       *paths.Paths
}

func (h *ScanHandler) validate() error {
	if h.CreatorUpdater == nil {
		return errors.New("internal error:CreatorUpdater is required")
	}
	if h.Paths == nil {
		return errors.New("internal error:Paths is required")
	}

	return nil
}

func (h *ScanHandler) Handle(ctx context.Context, f models.File, oldFile models.File) error {
	if err := h.validate(); err != nil {
		return err
	}

	AudioFile, ok := f.(*models.AudioFile)
	if !ok {
		return ErrNotAudioFile
	}

	// try to match the file to a audio
	existing, err := h.CreatorUpdater.FindByFileID(ctx, f.Base().ID)
	if err != nil {
		return fmt.Errorf("finding existing audio: %w", err)
	}

	if len(existing) == 0 {
		// try also to match file by fingerprints
		existing, err = h.CreatorUpdater.FindByFingerprints(ctx, AudioFile.Fingerprints.Filter(matchableFingerprintTypes...))
		if err != nil {
			return fmt.Errorf("finding existing audio by fingerprints: %w", err)
		}
	}

	if len(existing) > 0 {
		updateExisting := oldFile != nil
		if err := h.associateExisting(ctx, existing, AudioFile, updateExisting); err != nil {
			return err
		}
	} else {
		// create a new audio
		newAudio := models.NewAudio()

		logger.Infof("%s doesn't exist. Creating new audio...", f.Base().Path)

		cover := h.applyFileMetadata(ctx, &newAudio, f.Base().Path)

		if err := h.CreatorUpdater.Create(ctx, &newAudio, []models.FileID{AudioFile.ID}); err != nil {
			return fmt.Errorf("creating new audio: %w", err)
		}

		if len(cover) > 0 {
			if err := h.CreatorUpdater.UpdateCover(ctx, newAudio.ID, cover); err != nil {
				return fmt.Errorf("setting cover of new audio: %w", err)
			}
		}

		h.PluginCache.RegisterPostHooks(ctx, newAudio.ID, hook.AudioCreatePost, nil, nil)
	}

	return nil
}

// applyFileMetadata populates a new audio from the metadata embedded in its
// file, returning the embedded cover image if there is one. A file with missing
// or unreadable metadata is still a valid audio, so failures are only logged.
func (h *ScanHandler) applyFileMetadata(ctx context.Context, newAudio *models.Audio, path string) []byte {
	if h.MetadataReader == nil {
		return nil
	}

	metadata, err := h.MetadataReader.ReadMetadata(ctx, path)
	if err != nil {
		logger.Warnf("error reading metadata of %q: %v", path, err)
		return nil
	}

	newAudio.Title = metadata.Title
	newAudio.Details = metadata.Details
	newAudio.Date = metadata.Date

	if err := h.linkExisting(ctx, newAudio, metadata); err != nil {
		logger.Warnf("error matching metadata of %q to existing objects: %v", path, err)
	}

	return metadata.Cover
}

func (h *ScanHandler) linkExisting(ctx context.Context, newAudio *models.Audio, metadata *Metadata) error {
	if h.NameFinder.Performer != nil && len(metadata.Artists) > 0 {
		performers, err := h.NameFinder.Performer.FindByNames(ctx, metadata.Artists, true)
		if err != nil {
			return fmt.Errorf("finding performers: %w", err)
		}
		if len(performers) > 0 {
			ids := make([]int, len(performers))
			for i, p := range performers {
				ids[i] = p.ID
			}
			newAudio.PerformerIDs = models.NewRelatedIDs(ids)
		}
	}

	if h.NameFinder.Tag != nil && len(metadata.Genres) > 0 {
		tags, err := h.NameFinder.Tag.FindByNames(ctx, metadata.Genres, true)
		if err != nil {
			return fmt.Errorf("finding tags: %w", err)
		}
		if len(tags) > 0 {
			ids := make([]int, len(tags))
			for i, t := range tags {
				ids[i] = t.ID
			}
			newAudio.TagIDs = models.NewRelatedIDs(ids)
		}
	}

	if h.NameFinder.Group != nil && metadata.Album != "" {
		group, err := h.NameFinder.Group.FindByName(ctx, metadata.Album, true)
		if err != nil {
			return fmt.Errorf("finding group: %w", err)
		}
		if group != nil {
			newAudio.Groups = models.NewRelatedGroupsAudio([]models.GroupsAudios{
				{GroupID: group.ID, AudioIndex: metadata.Track},
			})
		}
	}

	return nil
}

func (h *ScanHandler) associateExisting(ctx context.Context, existing []*models.Audio, f *models.AudioFile, updateExisting bool) error {
	for _, s := range existing {
		if err := s.LoadFiles(ctx, h.CreatorUpdater); err != nil {
			return err
		}

		found := false
		for _, sf := range s.Files.List() {
			if sf.ID == f.ID {
				found = true
				break
			}
		}

		if !found {
			logger.Infof("Adding %s to audio %s", f.Path, s.DisplayName())

			if err := h.CreatorUpdater.AddFileID(ctx, s.ID, f.ID); err != nil {
				return fmt.Errorf("adding file to audio: %w", err)
			}
		}

		if !found || updateExisting {
			// update updated_at time when file association or content changes
			audioPartial := models.NewAudioPartial()
			if _, err := h.CreatorUpdater.UpdatePartial(ctx, s.ID, audioPartial); err != nil {
				return fmt.Errorf("updating audio: %w", err)
			}

			h.PluginCache.RegisterPostHooks(ctx, s.ID, hook.AudioUpdatePost, nil, nil)
		}
	}

	return nil
}
