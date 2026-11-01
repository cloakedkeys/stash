package txn

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeManager struct {
	commitErr error
	locked    error
	calls     []string
}

func (m *fakeManager) Begin(ctx context.Context, writable bool) (context.Context, error) {
	m.calls = append(m.calls, "begin")
	return ctx, nil
}

func (m *fakeManager) Commit(ctx context.Context) error {
	m.calls = append(m.calls, "commit")
	return m.commitErr
}

func (m *fakeManager) Rollback(ctx context.Context) error {
	m.calls = append(m.calls, "rollback")
	return nil
}

func (m *fakeManager) IsLocked(err error) bool { return m.locked != nil && errors.Is(err, m.locked) }

// addHooks records which hooks run, in the same list as the manager's calls.
func (m *fakeManager) addHooks(ctx context.Context) {
	note := func(name string) MustFunc {
		return func(ctx context.Context) { m.calls = append(m.calls, name) }
	}
	AddPostCommitHook(ctx, note("post-commit"))
	AddPostRollbackHook(ctx, note("post-rollback"))
	AddPostCompleteHook(ctx, note("post-complete"))
}

func TestWithTxnCommits(t *testing.T) {
	m := &fakeManager{}
	err := WithTxn(context.Background(), m, func(ctx context.Context) error {
		m.addHooks(ctx)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"begin", "commit", "post-commit", "post-complete"}, m.calls)
}

func TestWithTxnFunctionFails(t *testing.T) {
	m := &fakeManager{}
	want := errors.New("failed")
	err := WithTxn(context.Background(), m, func(ctx context.Context) error {
		m.addHooks(ctx)
		return want
	})
	require.ErrorIs(t, err, want)
	assert.Equal(t, []string{"begin", "rollback", "post-rollback", "post-complete"}, m.calls)
}

// A save that fails has to look like one: to the caller, and to the hooks that
// finish or undo what was done, such as deleting files.
func TestWithTxnCommitFails(t *testing.T) {
	want := errors.New("disk full")
	m := &fakeManager{commitErr: want}
	err := WithTxn(context.Background(), m, func(ctx context.Context) error {
		m.addHooks(ctx)
		return nil
	})
	require.ErrorIs(t, err, want)
	assert.Equal(t, []string{"begin", "commit", "rollback", "post-rollback", "post-complete"}, m.calls)
}

func TestWithTxnPreCommitHookFails(t *testing.T) {
	want := errors.New("hook failed")
	m := &fakeManager{}
	err := WithTxn(context.Background(), m, func(ctx context.Context) error {
		m.addHooks(ctx)
		AddPreCommitHook(ctx, func(ctx context.Context) error { return want })
		return nil
	})
	require.ErrorIs(t, err, want)
	assert.Equal(t, []string{"begin", "rollback", "post-rollback", "post-complete"}, m.calls, "the save is never tried, and is cancelled")
}

func TestWithTxnPreCommitHookPanics(t *testing.T) {
	m := &fakeManager{}
	assert.Panics(t, func() {
		_ = WithTxn(context.Background(), m, func(ctx context.Context) error {
			AddPreCommitHook(ctx, func(ctx context.Context) error { panic("hook panicked") })
			return nil
		})
	})
	assert.Equal(t, []string{"begin", "rollback"}, m.calls)
}

// A locked database at commit is retried, the same as one locked earlier on.
func TestRetryerRetriesLockedCommit(t *testing.T) {
	locked := errors.New("database is locked")
	m := &fakeManager{commitErr: locked, locked: locked}
	attempts := 0
	r := Retryer{
		Manager: m,
		Retries: 3,
		OnFail: func(ctx context.Context, err error, attempt int) error {
			m.commitErr = nil
			return nil
		},
	}
	err := r.WithTxn(context.Background(), func(ctx context.Context) error {
		attempts++
		m.addHooks(ctx)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, []string{
		"begin", "commit", "rollback", "post-rollback",
		"begin", "commit", "post-commit", "post-complete",
	}, m.calls, "the first attempt's clean-up isn't run as if it were saved")
}
