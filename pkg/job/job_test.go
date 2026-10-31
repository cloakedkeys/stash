package job

import (
	"testing"
	"time"
)

func TestJobTimeElapsed(t *testing.T) {
	start := time.Now().Add(-time.Hour)
	end := start.Add(time.Minute)

	notStarted := Job{}
	if got := notStarted.TimeElapsed(); got != 0 {
		t.Errorf("job that hasn't started: got %v, want 0", got)
	}

	running := Job{StartTime: &start}
	if got := running.TimeElapsed(); got < time.Hour {
		t.Errorf("running job: got %v, want at least an hour", got)
	}

	finished := Job{StartTime: &start, EndTime: &end}
	if got := finished.TimeElapsed(); got != time.Minute {
		t.Errorf("finished job: got %v, want %v", got, time.Minute)
	}
}
