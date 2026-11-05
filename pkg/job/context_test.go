package job

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
)

// A queued job runs with what the starting request knew about its user: the
// user, the session login time and the API key fingerprint. Plugin tasks use
// them for the plugin's cookie and calls, so that a task queued with a key
// that is deleted before it starts is refused (#2122). The request may be
// finished by the time the job starts.
func TestQueuedJobKeepsStartingUser(t *testing.T) {
	m := NewManager()

	loginTime := time.Unix(0, 1700000000123456789)
	fp := session.APIKeyFingerprint("stored-key-hash")

	reqCtx, finishRequest := context.WithCancel(context.Background())
	reqCtx = session.SetCurrentUser(reqCtx, models.User{ID: 7, Username: "admin"})
	reqCtx = session.SetSessionLoginTime(reqCtx, loginTime)
	reqCtx = session.SetAPIKeyFingerprint(reqCtx, fp)

	// a job ahead in the queue holds the queued one back
	release := make(chan struct{})
	m.Add(context.Background(), "ahead", MakeJobExec(func(ctx context.Context, progress *Progress) error {
		<-release
		return nil
	}))

	got := make(chan *session.PluginStarter, 1)
	m.Add(reqCtx, "queued", MakeJobExec(func(ctx context.Context, progress *Progress) error {
		got <- session.GetPluginStarter(ctx)
		return nil
	}))

	// the request finishes while the job waits
	finishRequest()
	close(release)

	var s *session.PluginStarter
	select {
	case s = <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("queued job did not run")
	}

	if s == nil {
		t.Fatal("queued job has no current user")
	}
	if s.User.Username != "admin" || s.User.ID != 7 {
		t.Errorf("user = %+v, want admin (7)", s.User)
	}
	if !s.LoginTime.Equal(loginTime) {
		t.Errorf("login time = %v, want %v", s.LoginTime, loginTime)
	}
	if s.APIKeyFingerprint != fp {
		t.Errorf("api key fingerprint = %q, want %q", s.APIKeyFingerprint, fp)
	}
}
