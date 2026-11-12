package dlna

import (
	"testing"

	"github.com/anacrolix/dms/upnpav"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestPartDLNAResource(t *testing.T) {
	start, end := 3.25, 8.75
	s := &models.Scene{ID: 7, StartTime: &start, EndTime: &end}
	s.Files.SetPrimary(&models.VideoFile{BaseFile: &models.BaseFile{Size: 1200}, Duration: 12, BitRate: 800})
	part := sceneToContainer(s, "0", "host:1").(upnpav.Item).Res[0]
	require.Equal(t, "0:00:05", part.Duration)
	require.Zero(t, part.Size)
	require.Contains(t, part.ProtocolInfo, "DLNA.ORG_OP=00")
	s.StartTime, s.EndTime = nil, nil
	whole := sceneToContainer(s, "0", "host:1").(upnpav.Item).Res[0]
	require.Equal(t, "0:00:12", whole.Duration)
	require.Equal(t, uint64(1200), whole.Size)
	require.Contains(t, whole.ProtocolInfo, "DLNA.ORG_OP=01")
}
