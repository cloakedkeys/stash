package fsutil

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

type Cancellable interface {
	Cancel()
}

type LockContext struct {
	context.Context
	cancel context.CancelFunc

	cmdMutex sync.Mutex
	cmd      *lockedCommand
}

type lockedCommand struct {
	cmd  *exec.Cmd
	once sync.Once
	done chan struct{}
	err  error
}

func (c *lockedCommand) wait() error {
	c.once.Do(func() {
		c.err = c.cmd.Wait()
		close(c.done)
	})
	return c.err
}

// AttachCommand registers a started command and returns its wait function.
// Both the caller and cancellation must use this function so Cmd.Wait is only
// called once, including when a file is deleted during generation.
func (c *LockContext) AttachCommand(cmd *exec.Cmd) func() error {
	command := &lockedCommand{cmd: cmd, done: make(chan struct{})}
	c.cmdMutex.Lock()
	c.cmd = command
	c.cmdMutex.Unlock()
	return command.wait
}

func (c *LockContext) Cancel() {
	c.cancel()

	c.cmdMutex.Lock()
	command := c.cmd
	c.cmdMutex.Unlock()
	if command != nil {
		// wait for the process to die before returning
		// don't wait more than a few seconds
		go command.wait()

		select {
		case <-command.done:
			return
		case <-time.After(5 * time.Second):
			return
		}
	}
}

// ReadLockManager manages read locks on file paths.
type ReadLockManager struct {
	readLocks map[string][]*LockContext
	mutex     sync.RWMutex
}

// NewReadLockManager creates a new ReadLockManager.
func NewReadLockManager() *ReadLockManager {
	return &ReadLockManager{
		readLocks: make(map[string][]*LockContext),
	}
}

// ReadLock adds a pending file read lock for fn to its storage, returning a context and cancel function.
// Per standard WithCancel usage, cancel must be called when the lock is freed.
func (m *ReadLockManager) ReadLock(ctx context.Context, fn string) *LockContext {
	retCtx, cancel := context.WithCancel(ctx)

	// if Cancellable, call Cancel() when cancelled
	cancellable, ok := ctx.(Cancellable)
	if ok {
		origCancel := cancel
		cancel = func() {
			origCancel()
			cancellable.Cancel()
		}
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	locks := m.readLocks[fn]

	cc := &LockContext{
		Context: retCtx,
		cancel:  cancel,
	}
	m.readLocks[fn] = append(locks, cc)

	go m.waitAndUnlock(fn, cc)

	return cc
}

func (m *ReadLockManager) waitAndUnlock(fn string, cc *LockContext) {
	<-cc.Done()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	locks := m.readLocks[fn]
	for i, v := range locks {
		if v == cc {
			m.readLocks[fn] = append(locks[:i], locks[i+1:]...)
			return
		}
	}
}

// Cancel cancels all read lock contexts associated with fn.
func (m *ReadLockManager) Cancel(fn string) {
	m.mutex.RLock()
	locks := m.readLocks[fn]
	m.mutex.RUnlock()

	for _, l := range locks {
		l.Cancel()
		<-l.Done()
	}
}
