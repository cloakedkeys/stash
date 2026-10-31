package models

import (
	"fmt"
	"io"
	"strconv"
)

// ProjectionEnum describes how a video frame is mapped onto a sphere
// or other projection surface. Generic naming; agnostic of any
// particular player or device.
type ProjectionEnum string

const (
	ProjectionEnumFlat            ProjectionEnum = "FLAT"
	ProjectionEnumEquirectangular ProjectionEnum = "EQUIRECTANGULAR"
	ProjectionEnumFisheye         ProjectionEnum = "FISHEYE"
	ProjectionEnumFisheye190      ProjectionEnum = "FISHEYE_190"
	ProjectionEnumMKX200          ProjectionEnum = "MKX200"
	ProjectionEnumMKX200Raw       ProjectionEnum = "MKX200_RAW"
	ProjectionEnumMKX220          ProjectionEnum = "MKX220"
	ProjectionEnumVRCA220         ProjectionEnum = "VRCA220"
	ProjectionEnumRF52            ProjectionEnum = "RF52"
	ProjectionEnumDome            ProjectionEnum = "DOME"
	ProjectionEnumSphere          ProjectionEnum = "SPHERE"
	ProjectionEnumCubemap         ProjectionEnum = "CUBEMAP"
	ProjectionEnumRectilinear     ProjectionEnum = "RECTILINEAR"
)

var AllProjectionEnum = []ProjectionEnum{
	ProjectionEnumFlat,
	ProjectionEnumEquirectangular,
	ProjectionEnumFisheye,
	ProjectionEnumFisheye190,
	ProjectionEnumMKX200,
	ProjectionEnumMKX200Raw,
	ProjectionEnumMKX220,
	ProjectionEnumVRCA220,
	ProjectionEnumRF52,
	ProjectionEnumDome,
	ProjectionEnumSphere,
	ProjectionEnumCubemap,
	ProjectionEnumRectilinear,
}

func (e ProjectionEnum) IsValid() bool {
	switch e {
	case ProjectionEnumFlat, ProjectionEnumEquirectangular, ProjectionEnumFisheye,
		ProjectionEnumFisheye190, ProjectionEnumMKX200, ProjectionEnumMKX200Raw,
		ProjectionEnumMKX220, ProjectionEnumVRCA220, ProjectionEnumRF52,
		ProjectionEnumDome, ProjectionEnumSphere,
		ProjectionEnumCubemap, ProjectionEnumRectilinear:
		return true
	}
	return false
}

func (e ProjectionEnum) String() string {
	return string(e)
}

func (e *ProjectionEnum) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}
	*e = ProjectionEnum(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid ProjectionEnum", str)
	}
	return nil
}

func (e ProjectionEnum) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

// StereoModeEnum describes how a stereoscopic video is laid out
// across the frame. Generic naming.
type StereoModeEnum string

const (
	StereoModeEnumMono                    StereoModeEnum = "MONO"
	StereoModeEnumSBS                     StereoModeEnum = "SBS"
	StereoModeEnumTB                      StereoModeEnum = "TB"
	StereoModeEnumSBSRight                StereoModeEnum = "SBS_RIGHT"
	StereoModeEnumSBSSqueezedLeft         StereoModeEnum = "SBS_SQUEEZED_LEFT"
	StereoModeEnumSBSSqueezedRight        StereoModeEnum = "SBS_SQUEEZED_RIGHT"
	StereoModeEnumTBRight                 StereoModeEnum = "TB_RIGHT"
	StereoModeEnumTBSqueezedLeft          StereoModeEnum = "TB_SQUEEZED_LEFT"
	StereoModeEnumTBSqueezedRight         StereoModeEnum = "TB_SQUEEZED_RIGHT"
	StereoModeEnumCUV                     StereoModeEnum = "CUV"
	StereoModeEnumAlternatingFrames       StereoModeEnum = "AF"
	StereoModeEnumAlternatingFramesRight  StereoModeEnum = "AF_RIGHT"
	StereoModeEnumInterleavedRows         StereoModeEnum = "INTERLEAVED_ROWS"
	StereoModeEnumInterleavedRowsRight    StereoModeEnum = "INTERLEAVED_ROWS_RIGHT"
	StereoModeEnumInterleavedColumns      StereoModeEnum = "INTERLEAVED_COLUMNS"
	StereoModeEnumInterleavedColumnsRight StereoModeEnum = "INTERLEAVED_COLUMNS_RIGHT"
)

var AllStereoModeEnum = []StereoModeEnum{
	StereoModeEnumMono,
	StereoModeEnumSBS,
	StereoModeEnumTB,
	StereoModeEnumSBSRight,
	StereoModeEnumSBSSqueezedLeft,
	StereoModeEnumSBSSqueezedRight,
	StereoModeEnumTBRight,
	StereoModeEnumTBSqueezedLeft,
	StereoModeEnumTBSqueezedRight,
	StereoModeEnumCUV,
	StereoModeEnumAlternatingFrames,
	StereoModeEnumAlternatingFramesRight,
	StereoModeEnumInterleavedRows,
	StereoModeEnumInterleavedRowsRight,
	StereoModeEnumInterleavedColumns,
	StereoModeEnumInterleavedColumnsRight,
}

func (e StereoModeEnum) IsValid() bool {
	switch e {
	case StereoModeEnumMono, StereoModeEnumSBS, StereoModeEnumTB,
		StereoModeEnumSBSRight, StereoModeEnumSBSSqueezedLeft, StereoModeEnumSBSSqueezedRight,
		StereoModeEnumTBRight, StereoModeEnumTBSqueezedLeft, StereoModeEnumTBSqueezedRight,
		StereoModeEnumCUV, StereoModeEnumAlternatingFrames, StereoModeEnumAlternatingFramesRight,
		StereoModeEnumInterleavedRows, StereoModeEnumInterleavedRowsRight,
		StereoModeEnumInterleavedColumns, StereoModeEnumInterleavedColumnsRight:
		return true
	}
	return false
}

func (e StereoModeEnum) String() string {
	return string(e)
}

func (e *StereoModeEnum) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}
	*e = StereoModeEnum(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid StereoModeEnum", str)
	}
	return nil
}

func (e StereoModeEnum) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

// VRCorrections holds optional per-video position and color adjustments,
// as offered by VR players. All fields are optional; nil means "unset".
// Passthrough settings (chroma key, alpha packing) are not corrections
// and are not stored here.
type VRCorrections struct {
	HorizontalOffset *float64 `json:"horizontal_offset"`
	VerticalOffset   *float64 `json:"vertical_offset"`
	Brightness       *float64 `json:"brightness"`
	Contrast         *float64 `json:"contrast"`
	Saturation       *float64 `json:"saturation"`
}

// VRChromaKey describes one color key used for passthrough.
type VRChromaKey struct {
	Hue        *float64 `json:"hue"`
	Saturation *float64 `json:"saturation"`
	Value      *float64 `json:"value"`
	Threshold  *float64 `json:"threshold"`
	Opacity    *float64 `json:"opacity"`
}

// VRPassthrough contains player settings for chroma-key passthrough.
type VRPassthrough struct {
	ChromaKeys  []VRChromaKey `json:"chroma_keys"`
	AlphaPacked *bool         `json:"alpha_packed"`
}
