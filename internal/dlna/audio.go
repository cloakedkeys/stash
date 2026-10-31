package dlna

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/anacrolix/dms/dlna"
	"github.com/anacrolix/dms/upnpav"

	"github.com/stashapp/stash/pkg/audio"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

const (
	audioFolderID = "audio"
	// numeric object ids are scenes, so audio items need their own prefix
	audioItemPrefix = audioFolderID + "/item/"
	audioParam      = "audio"
)

func audioToContainer(a *models.Audio, parent string, host string) interface{} {
	id := strconv.Itoa(a.ID)

	iconURI := (&url.URL{
		Scheme:   "http",
		Host:     host,
		Path:     iconPath,
		RawQuery: url.Values{audioParam: {id}}.Encode(),
	}).String()

	item := upnpav.Item{
		Object: upnpav.Object{
			ID:          audioItemPrefix + id,
			Restricted:  1,
			ParentID:    parent,
			Title:       a.GetTitle(),
			Class:       "object.item.audioItem.musicTrack",
			Icon:        iconURI,
			AlbumArtURI: iconURI,
		},
		Res: make([]upnpav.Resource, 0, 2),
	}

	mimeType := "audio/mpeg"
	var (
		size     int64
		bitrate  uint
		duration int64
	)

	if f := a.Files.Primary(); f != nil {
		mimeType = audio.MimeType(f)
		size = f.Size
		bitrate = uint(f.BitRate)
		duration = int64(f.Duration)
	}

	item.Res = append(item.Res, upnpav.Resource{
		URL: (&url.URL{
			Scheme:   "http",
			Host:     host,
			Path:     resPath,
			RawQuery: url.Values{audioParam: {id}}.Encode(),
		}).String(),
		ProtocolInfo: fmt.Sprintf("http-get:*:%s:%s", mimeType, dlna.ContentFeatures{
			SupportRange: true,
		}.String()),
		Bitrate:  bitrate,
		Duration: formatDurationSexagesimal(time.Duration(duration) * time.Second),
		Size:     uint64(size),
	})

	item.Res = append(item.Res, upnpav.Resource{
		URL:          iconURI,
		ProtocolInfo: "http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_MED",
	})

	return item
}

// audioFindFilter returns the filter for a page of audios. Unlike videos, audio
// is browsed like a music library, so it is always sorted by title.
func audioFindFilter(page int) *models.FindFilterType {
	sort := "title"
	direction := models.SortDirectionEnumAsc
	return &models.FindFilterType{
		PerPage:   &pageSize,
		Page:      &page,
		Sort:      &sort,
		Direction: &direction,
	}
}

func (me *contentDirectoryService) queryAudioObjects(ctx context.Context, page int, host string) ([]interface{}, error) {
	r := me.repository

	audios, err := audio.Query(ctx, r.AudioFinder, nil, audioFindFilter(page))
	if err != nil {
		return nil, err
	}

	var objs []interface{}
	for _, a := range audios {
		if err := a.LoadPrimaryFile(ctx, r.FileGetter); err != nil {
			return nil, err
		}
		objs = append(objs, audioToContainer(a, audioFolderID, host))
	}

	return objs, nil
}

// getAudios returns the top level of the audio folder: the audios themselves,
// or folders of pages if there are too many to show at once.
func (me *contentDirectoryService) getAudios(host string) []interface{} {
	var objs []interface{}

	r := me.repository
	if err := r.WithReadTxn(context.TODO(), func(ctx context.Context) error {
		total, err := r.AudioFinder.QueryCount(ctx, nil, nil)
		if err != nil {
			return err
		}

		if total > pageSize {
			pages := int(math.Ceil(float64(total) / float64(pageSize)))
			for page := 1; page <= pages; page++ {
				objs = append(objs, makeStorageFolder(audioFolderID+"/page/"+strconv.Itoa(page), fmt.Sprintf("Page %d", page), audioFolderID))
			}
			return nil
		}

		objs, err = me.queryAudioObjects(ctx, 1, host)
		return err
	}); err != nil {
		logger.Errorf("[DLNA] error getting audios: %v", err)
	}

	return objs
}

func (me *contentDirectoryService) getAudioPage(page int, host string) []interface{} {
	var objs []interface{}

	r := me.repository
	if err := r.WithReadTxn(context.TODO(), func(ctx context.Context) error {
		var err error
		objs, err = me.queryAudioObjects(ctx, page, host)
		return err
	}); err != nil {
		logger.Errorf("[DLNA] error getting audio page %d: %v", page, err)
	}

	return objs
}

// audioIDFromObjectPath returns the audio id of an audio item's object id.
func audioIDFromObjectPath(path string) (int, bool) {
	idStr, found := strings.CutPrefix(path, audioItemPrefix)
	if !found {
		return 0, false
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, false
	}

	return id, true
}

func (me *contentDirectoryService) findAudio(ctx context.Context, id int) (*models.Audio, error) {
	a, err := me.repository.AudioFinder.Find(ctx, id)
	if err != nil || a == nil {
		return nil, err
	}

	if err := a.LoadPrimaryFile(ctx, me.repository.FileGetter); err != nil {
		return nil, err
	}

	return a, nil
}
