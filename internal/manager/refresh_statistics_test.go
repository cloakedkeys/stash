package manager

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/job"
	"github.com/stretchr/testify/require"
)

type fakeRefresher chan struct{}

func (f fakeRefresher) RefreshStatistics(ctx context.Context) error {
	f <- struct{}{}
	return nil
}

// #2030: the statistics are refreshed after each task, which is when many rows
// can have changed at once.
func TestRefreshStatisticsAfterJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobs := job.NewManager()
	defer jobs.Stop()
	refreshed := make(fakeRefresher, 10)
	refreshStatisticsAfterJobs(ctx, jobs, refreshed)

	ran := make(chan struct{})
	jobs.Add(ctx, "scan", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error {
		select {
		case <-refreshed:
			t.Error("refreshed while the task was still running")
		default:
		}
		close(ran)
		return nil
	}))

	<-ran
	select {
	case <-refreshed:
	case <-time.After(5 * time.Second):
		t.Fatal("statistics weren't refreshed after the task finished")
	}
	require.Empty(t, refreshed, "once per task")
}
