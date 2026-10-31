package video

import (
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/pkg/models"
)

type filenameVRMetadata struct {
	projection  *models.ProjectionEnum
	stereoMode  *models.StereoModeEnum
	alphaPacked *bool
}

func inferVRMetadataFromFilename(filename string) filenameVRMetadata {
	name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	tokens := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		switch r {
		case '_', '-', '.', ' ', '[', ']', '(', ')', '{', '}', '+':
			return true
		default:
			return false
		}
	})

	var ret filenameVRMetadata
	for _, token := range tokens {
		if ret.projection == nil {
			if value, ok := filenameProjection(token); ok {
				ret.projection = &value
			}
		}
		if ret.stereoMode == nil {
			if value, ok := filenameStereoMode(token); ok {
				ret.stereoMode = &value
			}
		}
		if token == "alpha" && ret.alphaPacked == nil {
			value := true
			ret.alphaPacked = &value
		}
	}
	return ret
}

func filenameProjection(token string) (models.ProjectionEnum, bool) {
	switch token {
	case "flat":
		return models.ProjectionEnumFlat, true
	case "180", "180x180", "dome":
		return models.ProjectionEnumDome, true
	case "360", "sphere":
		return models.ProjectionEnumSphere, true
	case "fisheye", "fish":
		return models.ProjectionEnumFisheye, true
	case "equirectangular", "eq":
		return models.ProjectionEnumEquirectangular, true
	case "cubemap", "cube":
		return models.ProjectionEnumCubemap, true
	case "rectilinear":
		return models.ProjectionEnumRectilinear, true
	case "fisheye190":
		return models.ProjectionEnumFisheye190, true
	case "mkx200":
		return models.ProjectionEnumMKX200, true
	case "mkx200raw":
		return models.ProjectionEnumMKX200Raw, true
	case "mkx220":
		return models.ProjectionEnumMKX220, true
	case "vrca220":
		return models.ProjectionEnumVRCA220, true
	case "rf52":
		return models.ProjectionEnumRF52, true
	default:
		return "", false
	}
}

func filenameStereoMode(token string) (models.StereoModeEnum, bool) {
	switch token {
	case "mono":
		return models.StereoModeEnumMono, true
	case "sbs", "sbsl", "lr", "3dh":
		return models.StereoModeEnumSBS, true
	case "sbsr", "rl":
		return models.StereoModeEnumSBSRight, true
	case "sbs2l":
		return models.StereoModeEnumSBSSqueezedLeft, true
	case "sbs2r":
		return models.StereoModeEnumSBSSqueezedRight, true
	case "tb", "tbl", "ab", "abl", "3dv", "overunder":
		return models.StereoModeEnumTB, true
	case "tbr", "abr", "bt", "ba", "underover":
		return models.StereoModeEnumTBRight, true
	case "tb2l", "ab2l":
		return models.StereoModeEnumTBSqueezedLeft, true
	case "tb2r", "ab2r":
		return models.StereoModeEnumTBSqueezedRight, true
	case "al", "af", "afl", "irl", "ic", "icl", "interleavedrows", "interleavedcolumns":
		if token == "al" || token == "af" || token == "afl" {
			return models.StereoModeEnumAlternatingFrames, true
		}
		if token == "icl" || token == "ic" || token == "interleavedcolumns" {
			return models.StereoModeEnumInterleavedColumns, true
		}
		return models.StereoModeEnumInterleavedRows, true
	case "ar", "afr", "irr", "icr", "interleavedrowsright", "interleavedcolumnsright":
		if token == "ar" || token == "afr" {
			return models.StereoModeEnumAlternatingFramesRight, true
		}
		if token == "icr" || token == "interleavedcolumnsright" {
			return models.StereoModeEnumInterleavedColumnsRight, true
		}
		return models.StereoModeEnumInterleavedRowsRight, true
	case "cuv":
		return models.StereoModeEnumCUV, true
	default:
		return "", false
	}
}

func applyFilenameVRMetadata(vf *models.VideoFile, metadata filenameVRMetadata) bool {
	changed := false
	if vf.Projection == nil && metadata.projection != nil {
		vf.Projection = metadata.projection
		changed = true
	}
	if vf.StereoMode == nil && metadata.stereoMode != nil {
		vf.StereoMode = metadata.stereoMode
		changed = true
	}
	if metadata.alphaPacked != nil && (vf.VRPassthrough == nil || vf.VRPassthrough.AlphaPacked == nil) {
		if vf.VRPassthrough == nil {
			vf.VRPassthrough = &models.VRPassthrough{}
		}
		vf.VRPassthrough.AlphaPacked = metadata.alphaPacked
		changed = true
	}
	return changed
}
