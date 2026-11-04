package manager

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/job"
)

// fakeRefresher counts refreshes. Each one waits for gate, if set.
type fakeRefresher struct {
	calls chan struct{}
	gate  chan struct{}
}

func (f *fakeRefresher) RefreshStatistics(ctx context.Context) error {
	f.calls <- struct{}{}
	if f.gate != nil {
		<-f.gate
	}
	return nil
}

func (f *fakeRefresher) next(t *testing.T) {
	t.Helper()
	select {
	case <-f.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("statistics weren't refreshed after the task finished")
	}
}

func (f *fakeRefresher) none(t *testing.T) {
	t.Helper()
	select {
	case <-f.calls:
		t.Fatal("unexpected refresh")
	case <-time.After(300 * time.Millisecond):
	}
}

func startRefreshing(t *testing.T, f *fakeRefresher) (context.Context, *job.Manager) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	jobs := job.NewManager()
	t.Cleanup(jobs.Stop)
	f.calls = make(chan struct{}, 10)
	refreshStatisticsAfterJobs(ctx, jobs, f)
	return ctx, jobs
}

func noop(ctx context.Context, progress *job.Progress) error { return nil }

// #2030: the statistics are refreshed once after each task, which is when many
// rows can have changed at once.
func TestRefreshStatisticsAfterJobs(t *testing.T) {
	f := &fakeRefresher{}
	ctx, jobs := startRefreshing(t, f)

	release := make(chan struct{})
	jobs.Add(ctx, "scan", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error {
		<-release
		return nil
	}))
	f.none(t) // not while it runs
	close(release)

	f.next(t)
	f.none(t) // once
}

// A task cancelled before it started changed nothing, so it's no reason to
// refresh.
func TestRefreshStatisticsSkipsUnstartedJobs(t *testing.T) {
	f := &fakeRefresher{}
	ctx, jobs := startRefreshing(t, f)

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
	f.none(t)

	close(release)
	f.next(t)
	f.none(t)
}

// Tasks that finish while a refresh is running share the next one.
func TestRefreshStatisticsSharedByTasksFinishingTogether(t *testing.T) {
	gate := make(chan struct{})
	f := &fakeRefresher{gate: gate}
	ctx, jobs := startRefreshing(t, f)

	jobs.Add(ctx, "first", job.MakeJobExec(noop))
	f.next(t) // and now held at the gate

	for _, name := range []string{"second", "third", "fourth"} {
		if err := jobs.AddAndWait(ctx, name, job.MakeJobExec(noop)); err != nil {
			t.Fatal(err)
		}
	}
	// AddAndWait can return just before the last removal is announced
	time.Sleep(100 * time.Millisecond)
	close(gate)

	f.next(t)
	f.none(t)
}
