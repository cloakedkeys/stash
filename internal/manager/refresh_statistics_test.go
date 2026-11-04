package manager

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/job"
	"github.com/stretchr/testify/require"
)

// fakeRefresher records the changedSince of each refresh.
type fakeRefresher chan time.Time

func (f fakeRefresher) RefreshStatistics(ctx context.Context, changedSince time.Time) error {
	f <- changedSince
	return nil
}

func (f fakeRefresher) next(t *testing.T) time.Time {
	t.Helper()
	select {
	case since := <-f:
		return since
	case <-time.After(5 * time.Second):
		t.Fatal("statistics weren't refreshed after the task finished")
		return time.Time{}
	}
}

func (f fakeRefresher) none(t *testing.T) {
	t.Helper()
	select {
	case since := <-f:
		t.Fatalf("unexpected refresh, since %v", since)
	case <-time.After(300 * time.Millisecond):
	}
}

func startRefreshing(t *testing.T) (context.Context, *job.Manager, fakeRefresher) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	jobs := job.NewManager()
	t.Cleanup(jobs.Stop)
	refreshed := make(fakeRefresher, 10)
	refreshStatisticsAfterJobs(ctx, jobs, refreshed)
	return ctx, jobs, refreshed
}

// #2030: the statistics are refreshed once after each task, which is when many
// rows can have changed at once, covering everything since the task started.
func TestRefreshStatisticsAfterJobs(t *testing.T) {
	ctx, jobs, refreshed := startRefreshing(t)

	before := time.Now()
	release := make(chan struct{})
	jobs.Add(ctx, "scan", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error {
		<-release
		return nil
	}))
	refreshed.none(t) // not while it runs
	close(release)

	since := refreshed.next(t)
	require.False(t, since.Before(before), "since the task started")
	require.False(t, since.After(time.Now()))
	refreshed.none(t) // once
}

// A task cancelled before it started changed nothing, so it's no reason to
// refresh.
func TestRefreshStatisticsSkipsUnstartedJobs(t *testing.T) {
	ctx, jobs, refreshed := startRefreshing(t)

	release := make(chan struct{})
	started := make(chan struct{})
	jobs.Add(ctx, "running", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error {
		close(started)
		<-release
		return nil
	}))
	<-started
	queued := jobs.Add(ctx, "queued", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error {
		t.Error("cancelled task ran")
		return nil
	}))
	jobs.CancelJob(queued)
	refreshed.none(t)

	close(release)
	refreshed.next(t)
	refreshed.none(t)
}
