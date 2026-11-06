//go:build integration

package sqlite

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	oexec "os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stillWaiting is how long a Stash that should be waiting is given before
// the test takes it as waiting.
const stillWaiting = 500 * time.Millisecond

// databaseAtVersion makes a database at version, in a build that ends at
// end, and returns its path.
func databaseAtVersion(t *testing.T, version, end uint) string {
	t.Helper()
	endBuildAt(t, end)
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	db := NewDatabase()
	db.dbPath = path
	m, err := NewMigrator(db)
	require.NoError(t, err)
	migrateTo(t, m, version)
	m.Close()
	return path
}

// stash is another Stash with the database open.
func stash(t *testing.T, path string) *Migrator {
	t.Helper()
	db := NewDatabase()
	db.dbPath = path
	m, err := NewMigrator(db)
	require.NoError(t, err)
	return m
}

// upgrade upgrades as the upgrade task does, in the background.
func upgrade(ctx context.Context, m *Migrator) <-chan error {
	done := make(chan error, 1)
	go func() {
		for m.CurrentSchemaVersion() < m.RequiredSchemaVersion() {
			if err := m.RunMigration(ctx, m.CurrentSchemaVersion()+1); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	return done
}

// blockedStep makes the pre migration of version wait until release is
// closed, the first time it runs, then return err. It counts the times it
// runs.
type blockedStep struct {
	started chan struct{}
	release chan struct{}
	runs    atomic.Int32
}

func blockStep(t *testing.T, version uint, err error) *blockedStep {
	t.Helper()
	s := &blockedStep{started: make(chan struct{}), release: make(chan struct{})}
	saved := preMigrations[version]
	preMigrations[version] = append(append([]customMigrationFunc{}, saved...), func(ctx context.Context, db *sqlx.DB) error {
		if s.runs.Add(1) == 1 {
			close(s.started)
			<-s.release
			return err
		}
		return nil
	})
	t.Cleanup(func() { preMigrations[version] = saved })
	return s
}

// holdUpgradeLock locks the upgrade lock file as another Stash does, without
// running anything. The returned function releases it.
func holdUpgradeLock(t *testing.T, path string) func() {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", "file:"+filepath.ToSlash(path)+".upgrade-lock")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	_, err = conn.Exec("BEGIN IMMEDIATE")
	require.NoError(t, err)
	released := false
	release := func() {
		if !released {
			released = true
			_, _ = conn.Exec("ROLLBACK")
			_ = conn.Close()
		}
	}
	t.Cleanup(release)
	return release
}

func requireWaiting(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("the second Stash didn't wait for the first; it finished with %v", err)
	case <-time.After(stillWaiting):
	}
}

func result(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("the second Stash is still waiting")
		return nil
	}
}

// A second Stash that upgrades while the first is upgrading waits until the
// first has finished, then takes the database as it is, without running any
// step itself (#2213).
func TestSQLiteSecondUpgradeWaitsForFirst(t *testing.T) {
	ctx := context.Background()
	path := databaseAtVersion(t, 106, 108)
	step := blockStep(t, 107, nil)

	first := stash(t, path)
	defer first.Close()
	firstDone := upgrade(ctx, first)
	<-step.started

	second := stash(t, path)
	defer second.Close()
	secondDone := upgrade(ctx, second)
	requireWaiting(t, secondDone)

	close(step.release)
	require.NoError(t, <-firstDone)
	first.Close()

	require.NoError(t, result(t, secondDone))
	assert.Equal(t, uint(108), second.CurrentSchemaVersion())
	assert.Equal(t, int32(1), step.runs.Load(), "times step 107 ran")
	version, dirty, _ := schemaState(t, path)
	assert.Equal(t, uint(108), version)
	assert.False(t, dirty)
}

// A second Stash that waited for the first runs nothing if the first stopped
// part way, and says so (#2213). Here a step of the first failed.
func TestSQLiteSecondUpgradeStopsWhenFirstFails(t *testing.T) {
	ctx := context.Background()
	path := databaseAtVersion(t, 106, 108)
	step := blockStep(t, 107, errors.New("step failed"))

	first := stash(t, path)
	defer first.Close()
	firstDone := upgrade(ctx, first)
	<-step.started

	second := stash(t, path)
	defer second.Close()
	secondDone := upgrade(ctx, second)
	requireWaiting(t, secondDone)

	close(step.release)
	require.Error(t, <-firstDone)
	first.Close()

	err := result(t, secondDone)
	require.ErrorIs(t, err, database.ErrUpgradeUnfinishedElsewhere)
	assert.Contains(t, err.Error(), "it is at schema version 106")
	assert.Equal(t, int32(1), step.runs.Load(), "times step 107 ran")
	version, dirty, _ := schemaState(t, path)
	assert.Equal(t, uint(106), version)
	assert.False(t, dirty)
}

// The same when the first was stopped during a step's SQL, leaving its version
// dirty, or during its follow-up step, leaving it to run. The second changes
// nothing, so the next start finishes the upgrade (#2213).
func TestSQLiteSecondUpgradeStopsWhenFirstStopsPartWay(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		state string
	}{
		{"during the SQL", "UPDATE schema_migrations SET version = 108, dirty = 1"},
		{"during the follow-up step", "UPDATE schema_migrations SET version = 108, dirty = 0; CREATE TABLE IF NOT EXISTS pending_post_migrations (version INTEGER PRIMARY KEY); INSERT INTO pending_post_migrations VALUES (108)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := databaseAtVersion(t, 107, 108)
			second := stash(t, path)
			defer second.Close()

			release := holdUpgradeLock(t, path)
			rawExec(t, path, tc.state)
			// the second read version 107 before the first began
			secondDone := make(chan error, 1)
			go func() { secondDone <- second.RunMigration(ctx, 108) }()
			requireWaiting(t, secondDone)
			// the first Stash ends without finishing
			release()

			require.ErrorIs(t, result(t, secondDone), database.ErrUpgradeUnfinishedElsewhere)
			version, _, _ := schemaState(t, path)
			assert.Equal(t, uint(108), version)
		})
	}
}

// The upgrade task keeps the lock after closing the migrator, until it has
// put its backup back, so a second Stash doesn't start meanwhile (#2232).
func TestSQLiteKeptUpgradeLockOutlivesMigrator(t *testing.T) {
	ctx := context.Background()
	path := databaseAtVersion(t, 106, 108)

	first := stash(t, path)
	require.NoError(t, first.RunMigration(ctx, 107))
	release := first.KeepUpgradeLock()
	first.Close()

	second := stash(t, path)
	defer second.Close()
	secondDone := upgrade(ctx, second)
	requireWaiting(t, secondDone)

	release()
	require.ErrorIs(t, result(t, secondDone), database.ErrUpgradeUnfinishedElsewhere)
}

// TestHelperHoldsUpgradeLock isn't a test: run by
// TestSQLiteUpgradeLockEndsWithProcess, it holds the upgrade lock until it is
// killed.
func TestHelperHoldsUpgradeLock(t *testing.T) {
	path := os.Getenv("STASH_TEST_HOLD_UPGRADE_LOCK")
	if path == "" {
		t.Skip("only run by TestSQLiteUpgradeLockEndsWithProcess")
	}
	holdUpgradeLock(t, path)
	fmt.Println("holding the upgrade lock")
	time.Sleep(time.Minute)
}

// A Stash that is killed while it holds the lock doesn't leave the next one
// waiting for good: the operating system releases the lock (#2213).
func TestSQLiteUpgradeLockEndsWithProcess(t *testing.T) {
	ctx := context.Background()
	path := databaseAtVersion(t, 107, 108)
	second := stash(t, path)
	defer second.Close()

	first := oexec.Command(os.Args[0], "-test.run=^TestHelperHoldsUpgradeLock$", "-test.v")
	first.Env = append(os.Environ(), "STASH_TEST_HOLD_UPGRADE_LOCK="+path)
	out, err := first.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, first.Start())
	defer func() { _ = first.Process.Kill(); _ = first.Wait() }()
	lines := bufio.NewScanner(out)
	for lines.Scan() && !strings.Contains(lines.Text(), "holding the upgrade lock") {
	}

	secondDone := upgrade(ctx, second)
	requireWaiting(t, secondDone)
	require.NoError(t, first.Process.Kill())

	// it waited, and the first changed nothing, so it doesn't run the upgrade
	require.ErrorIs(t, result(t, secondDone), database.ErrUpgradeUnfinishedElsewhere)
}

// A Stash that starts while another is part way through a step, after the
// version was marked dirty, waits for the step rather than setting the
// version back (#2213, review of #2236).
func TestSQLiteStartDuringStepWaitsForIt(t *testing.T) {
	ctx := context.Background()
	path := databaseAtVersion(t, 107, 108)

	// the first Stash, as it runs step 108: it holds the lock and has marked
	// the version dirty, but not yet begun the step's SQL
	release := holdUpgradeLock(t, path)
	rawExec(t, path, "UPDATE schema_migrations SET version = 108, dirty = 1")

	opened := make(chan error, 1)
	second := NewDatabase()
	go func() { opened <- second.Open(ctx, path) }()
	requireWaiting(t, opened)

	// the step's SQL, with its mark, then golang-migrate's own mark
	r, _, err := markSource{mustSource(t)}.ReadUp(108)
	require.NoError(t, err)
	var sqlText strings.Builder
	_, err = bufio.NewReader(r).WriteTo(&sqlText)
	_ = r.Close()
	require.NoError(t, err)
	rawExec(t, path, sqlText.String())
	release()

	require.NoError(t, result(t, opened))
	defer second.Close()
	assert.Equal(t, uint(108), second.schemaVersion)
	assert.False(t, second.needsMigration())
	version, dirty, _ := schemaState(t, path)
	assert.Equal(t, uint(108), version)
	assert.False(t, dirty)
}

// A Stash that starts while another runs the last follow-up step of its
// upgrade waits for it, rather than running the same step at the same time
// (#2213, as #2220 for PostgreSQL).
func TestSQLiteStartDuringFollowUpStepWaitsForIt(t *testing.T) {
	ctx := context.Background()
	path := databaseAtVersion(t, 108, 108)
	var runs atomic.Int32
	saved := postMigrations[108]
	postMigrations[108] = []customMigrationFunc{func(ctx context.Context, db *sqlx.DB) error {
		runs.Add(1)
		return nil
	}}
	t.Cleanup(func() { postMigrations[108] = saved })

	// the first Stash, running step 108's follow-up step
	release := holdUpgradeLock(t, path)
	rawExec(t, path, "CREATE TABLE IF NOT EXISTS pending_post_migrations (version INTEGER PRIMARY KEY); INSERT INTO pending_post_migrations VALUES (108)")

	opened := make(chan error, 1)
	second := NewDatabase()
	go func() { opened <- second.Open(ctx, path) }()
	requireWaiting(t, opened)

	// the first finishes it
	rawExec(t, path, "DELETE FROM pending_post_migrations")
	release()

	require.NoError(t, result(t, opened))
	defer second.Close()
	assert.Equal(t, int32(0), runs.Load(), "times the second Stash ran the follow-up step")
}

// A Stash with the database to itself opens it without touching the lock
// file, so it isn't slowed even when another process holds it.
func TestSQLiteHealthyOpenTakesNoLock(t *testing.T) {
	ctx := context.Background()
	path := databaseAtVersion(t, 108, 108)
	holdUpgradeLock(t, path)

	opened := make(chan error, 1)
	db := NewDatabase()
	go func() { opened <- db.Open(ctx, path) }()
	select {
	case err := <-opened:
		require.NoError(t, err)
		require.NoError(t, db.Close())
	case <-time.After(stillWaiting):
		t.Fatal("opening an up-to-date database waited for the upgrade lock")
	}
}

// Without a lock file the upgrade goes on as before, with a warning, rather
// than Stash refusing to start.
func TestSQLiteUpgradeWithoutLockFile(t *testing.T) {
	ctx := context.Background()
	path := databaseAtVersion(t, 107, 108)
	// a folder where the lock file goes, so it can't be opened
	if err := os.Remove(path + ".upgrade-lock"); !errors.Is(err, os.ErrNotExist) {
		require.NoError(t, err)
	}
	require.NoError(t, os.Mkdir(path+".upgrade-lock", 0o755))

	m := stash(t, path)
	defer m.Close()
	require.NoError(t, result(t, upgrade(ctx, m)))
	assert.Equal(t, uint(108), m.CurrentSchemaVersion())
}
