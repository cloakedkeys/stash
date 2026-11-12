package generate

// rangeSegment converts scene-relative time to file time and caps the segment
// at the scene end. Tasks resolve open ends from the probed file duration.
func (g Generator) rangeSegment(seconds, duration float64) (float64, float64) {
	start := 0.0
	if g.Range.Start != nil {
		start = *g.Range.Start
	}
	if g.Range.End != nil {
		duration = min(duration, *g.Range.End-start-seconds)
	}
	return start + seconds, duration
}
