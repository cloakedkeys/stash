package image

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func zipGIF(t *testing.T, data []byte) (models.ZipFS, *models.BaseFile, string) {
	t.Helper()
	standalone := filepath.Join(t.TempDir(), "test.gif")
	require.NoError(t, os.WriteFile(standalone, data, 0600))
	path := filepath.Join(t.TempDir(), "images.zip")
	out, err := os.Create(path)
	require.NoError(t, err)
	zw := zip.NewWriter(out)
	w, err := zw.Create("test.gif")
	require.NoError(t, err)
	_, err = w.Write(data)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, out.Close())
	info, err := os.Stat(path)
	require.NoError(t, err)
	zf := &models.BaseFile{ID: 1, Path: path, Size: info.Size()}
	zfs, err := (&file.OsFS{}).OpenZip(path, info.Size())
	require.NoError(t, err)
	t.Cleanup(func() { _ = zfs.Close() })
	return zfs, &models.BaseFile{
		Path: filepath.Join(path, "test.gif"), Basename: "test.gif", Size: int64(len(data)),
		DirEntry: models.DirEntry{ZipFileID: &zf.ID, ZipFile: zf},
	}, standalone
}

func TestZipGIFMetadata(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 24, 16), color.Palette{color.Black, color.White})
	for _, delays := range [][]int{{0}, {20}, {20, 30}, {0, 0}, {1, 1}, {7, 12, 23}} {
		t.Run(fmt.Sprint(delays), func(t *testing.T) {
			var data bytes.Buffer
			g := &gif.GIF{Delay: delays}
			for range delays {
				g.Image = append(g.Image, frame)
			}
			require.NoError(t, gif.EncodeAll(&data, g))
			zfs, base, standalone := zipGIF(t, data.Bytes())
			d := &Decorator{}
			got, err := d.Decorate(context.Background(), zfs, base)
			require.NoError(t, err)
			vf, ok := got.(*models.VideoFile)
			require.True(t, ok, "GIF with %d frames scanned as %T", len(delays), got)
			require.Same(t, base, vf.BaseFile)
			require.Equal(t, "gif", vf.Format)
			require.Equal(t, "gif", vf.VideoCodec)
			require.Equal(t, 24, vf.Width)
			require.Equal(t, 16, vf.Height)
			ticks := 0
			for _, delay := range delays {
				if delay == 0 {
					delay = 10
				}
				ticks += delay
			}
			require.Equal(t, float64(ticks)/100, vf.Duration)
			require.False(t, d.IsMissingMetadata(context.Background(), zfs, vf))

			// An already saved ImageFile retains its identity and type, so
			// rescans cannot leave both subtype rows in either database store.
			base.ID = 42
			legacy, err := d.Decorate(context.Background(), zfs, &models.ImageFile{BaseFile: base})
			require.NoError(t, err)
			require.IsType(t, &models.ImageFile{}, legacy)
			require.Same(t, base, legacy.Base())

			t.Run("standalone parity", func(t *testing.T) {
				if delays[0] == 0 {
					// Go's encoder writes no timing block for a zero delay. ffprobe then
					// reports no duration, while browsers play it at the default speed
					// that the ZIP path uses.
					t.Skip("ffprobe has no duration for a GIF without timing blocks")
				}
				probePath, err := exec.LookPath("ffprobe")
				if err != nil {
					t.Skip("ffprobe is required to compare standalone GIF metadata")
				}
				d.FFProbe = ffmpeg.NewFFProbe(probePath)
				outside, err := d.Decorate(context.Background(), &file.OsFS{}, &models.BaseFile{Path: standalone})
				require.NoError(t, err)
				require.IsType(t, &models.VideoFile{}, outside, "including a one-frame GIF")
				want := outside.(*models.VideoFile)
				require.Equal(t, want.Format, vf.Format)
				require.Equal(t, want.Width, vf.Width)
				require.Equal(t, want.Height, vf.Height)
				require.InDelta(t, want.Duration, vf.Duration, 0.02)
			})
		})
	}
}

func TestZipGIFLargeCanvas(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black, color.White})
	var data bytes.Buffer
	require.NoError(t, gif.Encode(&data, frame, nil))
	// A huge logical screen with a tiny frame must not allocate its pixels.
	binary.LittleEndian.PutUint16(data.Bytes()[6:8], 65535)
	binary.LittleEndian.PutUint16(data.Bytes()[8:10], 65535)
	zfs, base, _ := zipGIF(t, data.Bytes())
	got, err := (&Decorator{}).Decorate(context.Background(), zfs, base)
	require.NoError(t, err)
	vf, ok := got.(*models.VideoFile)
	require.True(t, ok)
	require.Equal(t, 65535, vf.Width)
	require.Equal(t, 65535, vf.Height)
}

type existingGIFFingerprints struct{}

func (existingGIFFingerprints) CalculateFingerprints(f *models.BaseFile, o file.Opener, useExisting bool) ([]models.Fingerprint, error) {
	return f.Fingerprints, nil
}

func TestRescanKeepsExistingZipGIFImageFile(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	var data bytes.Buffer
	require.NoError(t, gif.Encode(&data, frame, nil))
	zfs, base, _ := zipGIF(t, data.Bytes())
	base.ID = 42
	existing := &models.ImageFile{BaseFile: base, Format: "gif", Width: 2, Height: 2}
	db := mocks.NewDatabase()
	db.File.On("FindByPath", mock.Anything, base.Path, true).Return(existing, nil)
	db.File.On("Update", mock.Anything, mock.MatchedBy(func(f models.File) bool {
		_, ok := f.(*models.ImageFile)
		return ok && f.Base() == base && f.Base().ID == 42
	})).Return(nil).Once()
	scanner := file.Scanner{
		Repository:            file.NewRepository(db.Repository()),
		FingerprintCalculator: existingGIFFingerprints{},
		FileDecorators:        []file.Decorator{&Decorator{}},
		HandlerRequiredFilters: []file.Filter{file.FilterFunc(func(context.Context, models.File) bool {
			return false
		})},
	}
	for _, force := range []bool{false, true} {
		scanner.Rescan = force
		scannedBase := *base
		result, err := scanner.ScanFile(context.Background(), file.ScannedFile{BaseFile: &scannedBase, FS: zfs})
		require.NoError(t, err)
		require.IsType(t, &models.ImageFile{}, result.File)
		require.Equal(t, models.FileID(42), result.File.Base().ID)
		require.False(t, result.New)
	}
	db.File.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	db.AssertExpectations(t)
}
