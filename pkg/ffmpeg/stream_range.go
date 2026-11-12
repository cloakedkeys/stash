package ffmpeg

// StreamRange is the scene's range in its primary file. It never changes the
// shared file metadata. A zero value preserves whole-file streaming.
type StreamRange struct {
	Start *float64
	End   *float64
}

func (r StreamRange) IsPart() bool { return r.Start != nil || r.End != nil }

func (r StreamRange) start() float64 {
	if r.Start != nil {
		return *r.Start
	}
	return 0
}

// Duration returns the scene length using the file duration for an open end.
func (r StreamRange) Duration(fileDuration float64) float64 {
	end := fileDuration
	if r.End != nil {
		end = *r.End
	}
	return max(0, end-r.start())
}
