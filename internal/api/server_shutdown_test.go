package api

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// startShutdownServer serves handler on a local port, set up as Initialize
// sets up the real server but with the given timeout and migration wait.
func startShutdownServer(t *testing.T, handler http.Handler, timeout time.Duration, stopMigrations func()) (*Server, string) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{Server: http.Server{Handler: handler}}
	s.prepareShutdown(timeout, stopMigrations)

	go func() { _ = s.Serve(l) }()

	return s, "http://" + l.Addr().String()
}

// shutdownWithin runs s.Shutdown and fails the test if it hasn't returned
// within limit. It returns how long Shutdown took.
func shutdownWithin(t *testing.T, s *Server, limit time.Duration) time.Duration {
	t.Helper()

	start := time.Now()
	done := make(chan struct{})
	go func() {
		s.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		return time.Since(start)
	case <-time.After(limit):
		t.Fatalf("Shutdown had not returned after %s", limit)
		return 0
	}
}

// A request that never finishes doesn't keep Stash from stopping: shutdown
// waits for the timeout, then ends it (#2138).
func TestShutdownEndsRequestStillRunningAfterTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	// never returns until the test has finished, even if its request is ended
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	})

	const timeout = 500 * time.Millisecond
	s, url := startShutdownServer(t, handler, timeout, func() {})

	clientErr := make(chan error, 1)
	go func() {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
		}
		clientErr <- err
	}()
	<-started

	took := shutdownWithin(t, s, timeout+5*time.Second)
	if took < timeout {
		t.Errorf("Shutdown took %s, want it to wait the %s timeout first", took, timeout)
	}

	select {
	case err := <-clientErr:
		if err == nil {
			t.Error("the request still running was answered, want its connection closed")
		}
	case <-time.After(5 * time.Second):
		t.Error("the connection of the request still running was not closed")
	}
}

// A request that finishes within the timeout finishes normally, as before.
func TestShutdownWaitsForRequestFinishingInTime(t *testing.T) {
	started := make(chan struct{})
	var cancelled atomic.Bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		cancelled.Store(r.Context().Err() != nil)
		_, _ = io.WriteString(w, "done")
	})

	const timeout = 5 * time.Second
	s, url := startShutdownServer(t, handler, timeout, func() {
		t.Error("waited for migrations, want that only after the timeout")
	})

	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := http.Get(url)
		if err != nil {
			got <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		got <- result{string(b), err}
	}()
	<-started

	took := shutdownWithin(t, s, timeout)
	if took >= timeout {
		t.Errorf("Shutdown took %s, want it to return when the request finished", took)
	}

	r := <-got
	if r.err != nil || r.body != "done" {
		t.Errorf("request got %q, %v; want \"done\", no error", r.body, r.err)
	}
	if cancelled.Load() {
		t.Error("the request's context was cancelled while it was finishing")
	}
}

// A request still running after the timeout isn't ended until a running
// database migration has finished, such as the database build of the setup
// page, which runs inside its request (#2103).
func TestShutdownWaitsForMigrationBeforeEndingRequests(t *testing.T) {
	started := make(chan struct{})
	ended := make(chan struct{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(ended)
	})

	migrationDone := make(chan struct{})
	waiting := make(chan struct{})
	stopMigrations := func() {
		close(waiting)
		<-migrationDone
	}

	const timeout = 200 * time.Millisecond
	s, url := startShutdownServer(t, handler, timeout, stopMigrations)

	go func() {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
		}
	}()
	<-started

	shutdownDone := make(chan struct{})
	go func() {
		s.Shutdown()
		close(shutdownDone)
	}()

	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown didn't wait for migrations after the timeout")
	}

	select {
	case <-ended:
		t.Fatal("the request was ended while a migration was running")
	case <-shutdownDone:
		t.Fatal("Shutdown returned while a migration was running")
	case <-time.After(300 * time.Millisecond):
	}

	close(migrationDone)

	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the request wasn't ended once the migration had finished")
	}
	select {
	case <-shutdownDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown didn't return once the migration had finished")
	}
}

// Shutdown doesn't wait for connections taken over from the server, such as
// GraphQL subscriptions over a websocket, but it ends them.
func TestShutdownEndsTakenOverConnections(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: test\r\nConnection: Upgrade\r\n\r\n")
		_ = rw.Flush()
		// as gqlgen does, end the subscription when the request context ends
		<-r.Context().Done()
	})

	const timeout = 5 * time.Second
	s, url := startShutdownServer(t, handler, timeout, func() {})

	conn, err := net.Dial("tcp", strings.TrimPrefix(url, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n")

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status %d, want 101", resp.StatusCode)
	}

	took := shutdownWithin(t, s, timeout)
	if took >= timeout {
		t.Errorf("Shutdown took %s, want it not to wait for the taken-over connection", took)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := br.ReadByte(); err != io.EOF {
		t.Errorf("reading the taken-over connection after Shutdown got %v, want EOF", err)
	}
}
