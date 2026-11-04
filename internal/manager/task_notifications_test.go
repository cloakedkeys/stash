package manager

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/job"
	"github.com/stretchr/testify/require"
)

func startNotifying(t *testing.T, enabled func() bool) (context.Context, *job.Manager, chan string) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	jobs := job.NewManager()
	t.Cleanup(jobs.Stop)
	sent := make(chan string, 10)
	notifyTasksFinished(ctx, jobs, enabled, func(title, text string) { sent <- title + ": " + text })
	return ctx, jobs, sent
}

func nextNotification(t *testing.T, sent chan string) string {
	t.Helper()
	select {
	case msg := <-sent:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("no notification")
		return ""
	}
}

func noNotification(t *testing.T, sent chan string) {
	t.Helper()
	select {
	case msg := <-sent:
		t.Fatalf("unexpected notification %q", msg)
	case <-time.After(300 * time.Millisecond):
	}
}

// #2033: cancelling a queued task used to stop all later "Task finished"
// notifications until Stash restarted.
func TestNotificationsAfterCancelledQueuedTask(t *testing.T) {
	ctx, jobs, sent := startNotifying(t, func() bool { return true })

	release := make(chan struct{})
	started := make(chan struct{})
	jobs.Add(ctx, "Scanning...", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error {
		close(started)
		<-release
		return nil
	}))
	<-started
	queued := jobs.Add(ctx, "Generating...", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error {
		t.Error("cancelled task ran")
		return nil
	}))
	jobs.CancelJob(queued)
	noNotification(t, sent) // nothing to report for the cancelled one

	close(release)
	msg := nextNotification(t, sent)
	require.True(t, strings.HasPrefix(msg, `Task Finished: Task "Scanning" finished in `), msg)

	// and the ones after that
	require.NoError(t, jobs.AddAndWait(ctx, "Auto tagging...", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error { return nil })))
	msg = nextNotification(t, sent)
	require.True(t, strings.HasPrefix(msg, `Task Finished: Task "Auto tagging" finished in `), msg)
}

// #2036: a task that failed or was stopped used to be reported as finished.
func TestNotificationsSayFailed(t *testing.T) {
	ctx, jobs, sent := startNotifying(t, func() bool { return true })
	require.Error(t, jobs.AddAndWait(ctx, "Scanning...", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error {
		return errors.New("disk unplugged")
	})))
	msg := nextNotification(t, sent)
	require.True(t, strings.HasPrefix(msg, `Task Failed: Task "Scanning" failed after `), msg)
}

func TestNotificationsSayStopped(t *testing.T) {
	ctx, jobs, sent := startNotifying(t, func() bool { return true })
	started := make(chan struct{})
	id := jobs.Add(ctx, "Scanning...", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error {
		close(started)
		<-ctx.Done()
		return nil
	}))
	<-started
	jobs.CancelJob(id)
	msg := nextNotification(t, sent)
	require.True(t, strings.HasPrefix(msg, `Task Stopped: Task "Scanning" was stopped after `), msg)
}

func TestNotificationsOff(t *testing.T) {
	ctx, jobs, sent := startNotifying(t, func() bool { return false })
	require.NoError(t, jobs.AddAndWait(ctx, "Scanning...", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error { return nil })))
	noNotification(t, sent)
}
