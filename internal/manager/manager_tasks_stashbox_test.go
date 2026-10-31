package manager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/stashbox"
)

func waitForFingerprintJob(t *testing.T, manager *job.Manager, id int) *job.Job {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		result := manager.GetJob(id)
		if result != nil && (result.Status == job.StatusFinished || result.Status == job.StatusFailed || result.Status == job.StatusCancelled) {
			return result
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fingerprint job %d did not finish", id)
	return nil
}

func TestSubmitStashBoxFingerprintsJobLifecycle(t *testing.T) {
	t.Run("completes successfully", func(t *testing.T) {
		manager := job.NewManager()
		t.Cleanup(manager.Stop)

		exec := newSubmitStashBoxFingerprintsJob(
			"mock endpoint",
			func(context.Context) ([]*models.Scene, error) { return nil, nil },
			func(context.Context, []*models.Scene, *job.Progress) (*stashbox.FingerprintSubmissionResult, error) {
				return &stashbox.FingerprintSubmissionResult{Total: 2, Succeeded: 2}, nil
			},
		)
		id := manager.Add(context.Background(), "mock fingerprint submission", exec)

		result := waitForFingerprintJob(t, manager, id)
		if result.Status != job.StatusFinished {
			t.Fatalf("job status = %s, want %s", result.Status, job.StatusFinished)
		}
	})

	t.Run("reports submission failure", func(t *testing.T) {
		manager := job.NewManager()
		t.Cleanup(manager.Stop)

		exec := newSubmitStashBoxFingerprintsJob(
			"mock endpoint",
			func(context.Context) ([]*models.Scene, error) { return nil, nil },
			func(context.Context, []*models.Scene, *job.Progress) (*stashbox.FingerprintSubmissionResult, error) {
				return nil, errors.New("mock submission failure")
			},
		)
		id := manager.Add(context.Background(), "mock fingerprint submission", exec)

		result := waitForFingerprintJob(t, manager, id)
		if result.Status != job.StatusFailed {
			t.Fatalf("job status = %s, want %s", result.Status, job.StatusFailed)
		}
		if result.Error == nil || *result.Error != "mock submission failure" {
			t.Fatalf("job error = %v, want mock submission failure", result.Error)
		}
	})

	t.Run("preserves canceled queue status", func(t *testing.T) {
		manager := job.NewManager()
		t.Cleanup(manager.Stop)
		started := make(chan struct{})

		exec := newSubmitStashBoxFingerprintsJob(
			"mock endpoint",
			func(context.Context) ([]*models.Scene, error) { return nil, nil },
			func(ctx context.Context, _ []*models.Scene, _ *job.Progress) (*stashbox.FingerprintSubmissionResult, error) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			},
		)
		id := manager.Add(context.Background(), "mock fingerprint submission", exec)
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("fingerprint submission did not start")
		}

		manager.CancelJob(id)
		result := waitForFingerprintJob(t, manager, id)
		if result.Status != job.StatusCancelled {
			t.Fatalf("job status = %s, want %s", result.Status, job.StatusCancelled)
		}
	})
}
