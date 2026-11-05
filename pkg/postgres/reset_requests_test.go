//go:build integration

package postgres

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/require"
)

func newResetTestDatabase(ctx context.Context, t *testing.T) *Database {
	t.Helper()
	db := NewDatabase()
	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		t.Skip("PGSQL_TEST is required")
	}
	// Reset drops the public tables, so use a separate throwaway database.
	admin, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	require.NoError(t, err)
	name := fmt.Sprintf("reset_requests_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.Exec("DROP DATABASE " + name)
		require.NoError(t, err)
		require.NoError(t, admin.Close())
	})
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/" + name
	config.InitializeEmpty()
	require.NoError(t, db.Open(ctx, u.String()))
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	return db
}

func TestRequestDuringReset(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := newResetTestDatabase(ctx, t)

	// Use a real account so the request also checks account restoration.
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		return db.User().Create(ctx, &models.User{Username: "preserved-account"}, "")
	}))

	// Pause Reset while it reads the accounts, before it removes the database.
	// WaitCount proves Reset has started without adding a production test hook.
	db.writeDB.SetMaxOpenConns(1)
	conn, err := db.writeDB.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	waitCount := db.writeDB.Stats().WaitCount
	resetDone := make(chan error, 1)
	go func() { resetDone <- db.Reset() }()
	require.Eventually(t, func() bool {
		return db.writeDB.Stats().WaitCount > waitCount
	}, 5*time.Second, time.Millisecond)

	started := make(chan struct{})
	requestDone := make(chan struct{})
	var response *http.Response
	var requestErr error
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		err := txn.WithReadTxn(r.Context(), db, func(ctx context.Context) error {
			var name string
			if err := dbWrapper.Get(ctx, &name, "SELECT username FROM users WHERE username = 'preserved-account'"); err != nil {
				return err
			}
			if name != "preserved-account" {
				return fmt.Errorf("account was not restored: %q", name)
			}
			return nil
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/graphql", nil)
	require.NoError(t, err)
	go func() {
		defer close(requestDone)
		response, requestErr = server.Client().Do(request)
	}()
	<-started
	select {
	case <-requestDone:
		t.Error("request completed before the database reset finished")
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, conn.Close())
	select {
	case err := <-resetDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("reset did not finish:", ctx.Err())
	}
	select {
	case <-requestDone:
		require.NoError(t, requestErr)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	case <-ctx.Done():
		t.Fatal("request did not finish:", ctx.Err())
	}
}

func TestResetWaitsForDatabaseWork(t *testing.T) {
	for _, scope := range []string{"read transaction", "write transaction", "database callback"} {
		t.Run(scope, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			db := newResetTestDatabase(ctx, t)

			var workCtx context.Context
			var release func()
			var err error
			if scope == "database callback" {
				workCtx, release, err = db.WithDatabase(ctx)
			} else {
				workCtx, err = db.Begin(ctx, scope == "write transaction")
				release = sync.OnceFunc(func() { require.NoError(t, db.Rollback(workCtx)) })
			}
			require.NoError(t, err)
			defer release()

			resetDone := make(chan error, 1)
			go func() { resetDone <- db.Reset() }()
			// A pending writer rejects TryRLock even while existing readers run.
			require.Eventually(t, func() bool {
				if db.resetMu.TryRLock() {
					db.resetMu.RUnlock()
					return false
				}
				return true
			}, 5*time.Second, time.Millisecond)

			// Nested use must reuse the guard rather than wait for the reset
			// which is itself waiting for this work to finish.
			workDone := make(chan error, 1)
			go func() {
				workDone <- txn.WithDatabase(workCtx, db, func(ctx context.Context) error {
					var count int
					if err := dbWrapper.Get(ctx, &count, "SELECT COUNT(*) FROM users"); err != nil {
						return err
					}
					if scope == "database callback" {
						return txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
							return dbWrapper.Get(ctx, &count, "SELECT COUNT(*) FROM users")
						})
					}
					return nil
				})
			}()
			select {
			case err := <-workDone:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal("existing work blocked behind reset:", ctx.Err())
			}
			select {
			case err := <-resetDone:
				t.Fatalf("reset finished before database work was released: %v", err)
			default:
			}

			release()
			select {
			case err := <-resetDone:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal("reset did not finish:", ctx.Err())
			}
			require.NoError(t, db.Ready())
		})
	}
}
