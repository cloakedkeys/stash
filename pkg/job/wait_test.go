package job

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAddAndWaitQueuesAndReturnsResult(t *testing.T) {
	for _, cancelQueued := range []bool{false, true} {
		t.Run(map[bool]string{false: "completes", true: "cancelled while queued"}[cancelQueued], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			m := NewManager()
			defer m.Stop()
			sub := m.Subscribe(ctx)
			gate := make(chan struct{})
			gateClosed := false
			defer func() {
				if !gateClosed {
					close(gate)
				}
			}()
			started := make(chan struct{})
			m.Add(ctx, "first", MakeJobExec(func(context.Context, *Progress) error {
				close(started)
				<-gate
				return nil
			}))
			<-started
			ran := make(chan struct{}, 1)
			result := make(chan error, 1)
			go func() {
				result <- m.AddAndWait(ctx, "backup", MakeJobExec(func(context.Context, *Progress) error {
					ran <- struct{}{}
					return nil
				}))
			}()
			var queuedID int
			for queuedID == 0 {
				select {
				case j := <-sub.NewJob:
					if j.Description == "backup" {
						queuedID = j.ID
					}
				case <-ctx.Done():
					t.Fatal("backup was not queued")
				}
			}
			select {
			case <-ran:
				t.Fatal("backup overlapped the first task")
			default:
			}
			if cancelQueued {
				m.CancelJob(queuedID)
				if err := <-result; !errors.Is(err, context.Canceled) {
					t.Fatalf("got %v", err)
				}
			} else {
				close(gate)
				gateClosed = true
				if err := <-result; err != nil {
					t.Fatal(err)
				}
				select {
				case <-ran:
				default:
					t.Fatal("backup did not run")
				}
			}
		})
	}
}

func TestAddAndWaitReturnsFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := NewManager()
	defer m.Stop()
	err := m.AddAndWait(ctx, "backup", MakeJobExec(func(context.Context, *Progress) error { return errors.New("backup disk full") }))
	if err == nil || err.Error() != "job failed: backup disk full" {
		t.Fatalf("got %v", err)
	}
}
