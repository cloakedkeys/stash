package dlna

import (
	"testing"

	"github.com/anacrolix/dms/upnpav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/pkg/models"
)

func TestAudioToContainer(t *testing.T) {
	a := &models.Audio{ID: 7, Title: "Song"}
	a.Files = models.NewRelatedAudioFiles([]*models.AudioFile{
		{Format: "flac", Duration: 90, BitRate: 900, BaseFile: &models.BaseFile{Size: 1234}},
	})

	item, ok := audioToContainer(a, audioFolderID, "host:1").(upnpav.Item)
	require.True(t, ok)

	assert.Equal(t, "audio/item/7", item.ID)
	assert.Equal(t, "Song", item.Title)
	assert.Equal(t, "object.item.audioItem.musicTrack", item.Class)
	assert.Equal(t, audioFolderID, item.ParentID)

	require.Len(t, item.Res, 2)
	assert.Equal(t, "http://host:1/res?audio=7", item.Res[0].URL)
	assert.Contains(t, item.Res[0].ProtocolInfo, "audio/flac")
	assert.Equal(t, uint64(1234), item.Res[0].Size)
	assert.Equal(t, "0:01:30", item.Res[0].Duration)
}

func TestAudioIDFromObjectPath(t *testing.T) {
	id, ok := audioIDFromObjectPath("audio/item/12")
	assert.True(t, ok)
	assert.Equal(t, 12, id)

	for _, path := range []string{"12", "audio", "audio/page/2", "audio/item/x"} {
		_, ok := audioIDFromObjectPath(path)
		assert.False(t, ok, path)
	}
}
