package postgres

import (
	"context"
	"net"
	"testing"
	"time"
)

// A stop request during startup cancels the context used to open the
// database. Opening must then give up at once, even while the server doesn't
// answer, instead of waiting for the connection to time out (#2103).
func TestOpenGivesUpWhenCancelled(t *testing.T) {
	// a server that accepts connections and never answers
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		var conns []net.Conn
		for {
			c, err := l.Accept()
			if err != nil {
				for _, c := range conns {
					_ = c.Close()
				}
				return
			}
			conns = append(conns, c)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)

	dsn := "postgres://stash:stash@" + l.Addr().String() + "/stash?sslmode=disable&connect_timeout=60"
	db := NewDatabase()
	start := time.Now()
	err = db.Open(ctx, dsn)
	took := time.Since(start)

	if err == nil {
		t.Fatal("Open should fail against a server that doesn't answer")
	}
	if took > 10*time.Second {
		t.Errorf("Open took %v after it was cancelled; it should give up at once", took)
	}
}
