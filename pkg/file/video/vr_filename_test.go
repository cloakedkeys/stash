package video

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
)

func TestInferVRMetadataFromFilename(t *testing.T) {
	tests := []struct {
		name       string
		projection models.ProjectionEnum
		stereo     models.StereoModeEnum
		alpha      bool
	}{
		{"Movie_180_SBS2L.mkv", models.ProjectionEnumDome, models.StereoModeEnumSBSSqueezedLeft, false},
		{"Movie-360-tbr.mp4", models.ProjectionEnumSphere, models.StereoModeEnumTBRight, false},
		{"movie.mkv", "", "", false},
		{"clip_mkx200raw_ICR_alpha.mp4", models.ProjectionEnumMKX200Raw, models.StereoModeEnumInterleavedColumnsRight, true},
		{"lens.fisheye190.sbsr.mkv", models.ProjectionEnumFisheye190, models.StereoModeEnumSBSRight, false},
		{"clip_vrca220_icl.mp4", models.ProjectionEnumVRCA220, models.StereoModeEnumInterleavedColumns, false},
		{"clip_tb2r_mkx220.mkv", models.ProjectionEnumMKX220, models.StereoModeEnumTBSqueezedRight, false},
		{"clip_3dh_tb.mp4", "", models.StereoModeEnumSBS, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferVRMetadataFromFilename(tt.name)
			if tt.projection == "" {
				if got.projection != nil {
					t.Errorf("unexpected projection: %s", *got.projection)
				}
			} else if got.projection == nil || *got.projection != tt.projection {
				t.Errorf("projection = %v, want %s", got.projection, tt.projection)
			}
			if tt.stereo == "" {
				if got.stereoMode != nil {
					t.Errorf("unexpected stereo mode: %s", *got.stereoMode)
				}
			} else if got.stereoMode == nil || *got.stereoMode != tt.stereo {
				t.Errorf("stereo mode = %v, want %s", got.stereoMode, tt.stereo)
			}
			if tt.alpha && (got.alphaPacked == nil || !*got.alphaPacked) {
				t.Error("expected alpha-packed inference")
			} else if !tt.alpha && got.alphaPacked != nil {
				t.Errorf("unexpected alpha-packed inference: %v", *got.alphaPacked)
			}
		})
	}
}

func TestFilenameVRMetadataInferenceIsOptIn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip_180_sbs2l.mkv")
	videoFile := &models.VideoFile{
		BaseFile:   &models.BaseFile{Basename: filepath.Base(path), Path: path},
		Format:     "mp4",
		VideoCodec: "h264",
		AudioCodec: "aac",
		Width:      1920,
		Height:     1080,
		FrameRate:  30,
		Duration:   60,
		BitRate:    1000,
	}
	fs := &file.OsFS{}

	if (&Decorator{}).IsMissingMetadata(context.Background(), fs, videoFile) {
		t.Fatal("filename metadata should be ignored when inference is disabled")
	}

	decorator := &Decorator{InferVRMetadata: true}
	if !decorator.IsMissingMetadata(context.Background(), fs, videoFile) {
		t.Fatal("expected scan to detect unset metadata from filename when enabled")
	}
	applyFilenameVRMetadata(videoFile, inferVRMetadataFromFilename(videoFile.Basename))
	if decorator.IsMissingMetadata(context.Background(), fs, videoFile) {
		t.Fatal("inferred metadata should no longer be reported missing")
	}
}

func TestApplyFilenameVRMetadataOnlyFillsUnsetFields(t *testing.T) {
	projection := models.ProjectionEnumFlat
	stereo := models.StereoModeEnumMono
	alpha := false
	metadata := inferVRMetadataFromFilename("movie_360_sbs2l_alpha.mp4")
	file := &models.VideoFile{
		Projection: &projection,
		StereoMode: &stereo,
		VRPassthrough: &models.VRPassthrough{
			AlphaPacked: &alpha,
		},
	}

	if changed := applyFilenameVRMetadata(file, metadata); changed {
		t.Fatal("existing values should prevent filename inference")
	}
	if *file.Projection != projection || *file.StereoMode != stereo || *file.VRPassthrough.AlphaPacked != alpha {
		t.Fatal("existing VR metadata was overwritten")
	}

	file = &models.VideoFile{}
	if changed := applyFilenameVRMetadata(file, metadata); !changed {
		t.Fatal("expected unset VR values to be populated")
	}
	if *file.Projection != models.ProjectionEnumSphere || *file.StereoMode != models.StereoModeEnumSBSSqueezedLeft || !*file.VRPassthrough.AlphaPacked {
		t.Fatalf("unexpected inferred values: projection=%v stereo=%v passthrough=%+v", file.Projection, file.StereoMode, file.VRPassthrough)
	}
}

func TestPreserveVRMetadataOnRescan(t *testing.T) {
	projection := models.ProjectionEnumFlat
	stereoMode := models.StereoModeEnumMono
	brightness := 0.25
	alphaPacked := false
	hue := 120.0
	old := &models.VideoFile{
		Projection: &projection,
		StereoMode: &stereoMode,
		VRCorrections: &models.VRCorrections{
			Brightness: &brightness,
		},
		VRPassthrough: &models.VRPassthrough{
			AlphaPacked: &alphaPacked,
			ChromaKeys:  []models.VRChromaKey{{Hue: &hue}},
		},
	}
	decorated := &models.VideoFile{}

	preserveVRMetadata(decorated, old)
	applyFilenameVRMetadata(decorated, inferVRMetadataFromFilename("clip_360_sbs2l_alpha.mp4"))

	if *decorated.Projection != projection || *decorated.StereoMode != stereoMode {
		t.Fatal("rescan or filename inference overwrote existing projection/stereo metadata")
	}
	if decorated.VRCorrections != old.VRCorrections || decorated.VRPassthrough != old.VRPassthrough {
		t.Fatal("rescan did not preserve corrections and passthrough")
	}
}
