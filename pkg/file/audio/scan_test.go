package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestDecoratorDecoratesWaveFileWithFFProbe(t *testing.T) {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not available in PATH")
	}

	const (
		sampleRate = 8000
		dataSize   = sampleRate * 2
	)
	path := filepath.Join(t.TempDir(), "audio-scan.wav")
	var data bytes.Buffer
	require.NoError(t, binary.Write(&data, binary.LittleEndian, []byte("RIFF")))
	require.NoError(t, binary.Write(&data, binary.LittleEndian, uint32(36+dataSize)))
	require.NoError(t, binary.Write(&data, binary.LittleEndian, []byte("WAVEfmt ")))
	require.NoError(t, binary.Write(&data, binary.LittleEndian, uint32(16)))
	require.NoError(t, binary.Write(&data, binary.LittleEndian, uint16(1)))
	require.NoError(t, binary.Write(&data, binary.LittleEndian, uint16(1)))
	require.NoError(t, binary.Write(&data, binary.LittleEndian, uint32(sampleRate)))
	require.NoError(t, binary.Write(&data, binary.LittleEndian, uint32(sampleRate*2)))
	require.NoError(t, binary.Write(&data, binary.LittleEndian, uint16(2)))
	require.NoError(t, binary.Write(&data, binary.LittleEndian, uint16(16)))
	require.NoError(t, binary.Write(&data, binary.LittleEndian, []byte("data")))
	require.NoError(t, binary.Write(&data, binary.LittleEndian, uint32(dataSize)))
	require.NoError(t, binary.Write(&data, binary.LittleEndian, make([]byte, dataSize)))
	require.NoError(t, os.WriteFile(path, data.Bytes(), 0o600))

	input := &models.AudioFile{BaseFile: &models.BaseFile{Path: path}}
	decorator := Decorator{FFProbe: ffmpeg.NewFFProbe(ffprobePath)}
	decorated, err := decorator.Decorate(context.Background(), &file.OsFS{}, input)
	require.NoError(t, err)

	audioFile, ok := decorated.(*models.AudioFile)
	require.True(t, ok)
	require.Equal(t, "wav", audioFile.Format)
	require.Equal(t, "pcm_s16le", audioFile.AudioCodec)
	require.InDelta(t, 1.0, audioFile.Duration, 0.01)
	require.EqualValues(t, sampleRate, audioFile.SampleRate)
	require.InDelta(t, sampleRate*16, audioFile.BitRate, 500)
	require.False(t, decorator.IsMissingMetadata(context.Background(), &file.OsFS{}, audioFile))
}
