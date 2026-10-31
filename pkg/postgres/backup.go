package postgres

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/logger"
)

const (
	pgDumpTool    = "pg_dump"
	pgRestoreTool = "pg_restore"

	// where the existing schema is kept while a backup is restored
	restoreHoldingSchema = "stash_before_restore"
)

// the connection settings that the client tools have an environment variable for,
// other than those available from the parsed configuration
var clientToolEnvSettings = map[string]string{
	"sslmode":     "PGSSLMODE",
	"sslrootcert": "PGSSLROOTCERT",
	"sslcert":     "PGSSLCERT",
	"sslkey":      "PGSSLKEY",
}

var connectionSettingRE = regexp.MustCompile(`(?:^|[?&\s])(sslmode|sslrootcert|sslcert|sslkey)=([^&\s]+)`)

// clientToolEnv returns the environment for running a client tool against the
// database. The password goes here because arguments can be seen by other
// processes on the machine.
func clientToolEnv(dsn string) ([]string, string, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, "", fmt.Errorf("parsing database connection: %w", err)
	}

	env := append(os.Environ(),
		"PGHOST="+cfg.Host,
		"PGPORT="+strconv.Itoa(int(cfg.Port)),
		"PGUSER="+cfg.User,
		"PGPASSWORD="+cfg.Password,
	)

	for _, m := range connectionSettingRE.FindAllStringSubmatch(dsn, -1) {
		env = append(env, clientToolEnvSettings[m[1]]+"="+m[2])
	}

	return env, cfg.Database, nil
}

// runClientTool runs a client tool, returning database.ErrBackupToolNotFound if
// it isn't installed.
func runClientTool(env []string, stdin io.Reader, stdout io.Writer, name string, args ...string) error {
	path, err := osexec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%w: %s", database.ErrBackupToolNotFound, name)
	}

	var stderr bytes.Buffer
	cmd := osexec.Command(path, args...)
	cmd.Env = env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running %s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}

	return nil
}

// Backup writes a copy of the database to backupPath using pg_dump, which must
// be installed and at least the same version as the server. The file is only
// created at backupPath once it is complete.
func (db *Database) Backup(backupPath string) (err error) {
	env, dbName, err := clientToolEnv(db.dbPath)
	if err != nil {
		return err
	}

	tmpPath := backupPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating backup file: %w", err)
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	logger.Infof("Backing up database into %s", backupPath)

	if err := runClientTool(env, nil, f, pgDumpTool, "--format=custom", "--no-owner", "--no-privileges", "--dbname="+dbName); err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("writing backup file: %w", err)
	}

	return os.Rename(tmpPath, backupPath)
}

// RestoreFromBackup replaces the contents of the database with a backup created
// by Backup, using pg_restore. If the backup can't be restored, then the database
// is left as it was. The backup file is kept.
func (db *Database) RestoreFromBackup(backupPath string) error {
	env, dbName, err := clientToolEnv(db.dbPath)
	if err != nil {
		return err
	}

	// check for this first so that nothing is changed if it would fail
	if _, err := osexec.LookPath(pgRestoreTool); err != nil {
		return fmt.Errorf("%w: %s", database.ErrBackupToolNotFound, pgRestoreTool)
	}

	f, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("opening backup file: %w", err)
	}
	defer f.Close()

	logger.Infof("Restoring backup database %s", backupPath)

	// existing connections may have plans for the tables being replaced
	if err := db.Close(); err != nil {
		return fmt.Errorf("closing database: %w", err)
	}

	conn, err := db.open(false, true)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx := context.Background()

	// A backup from before a failed migration doesn't include what that migration
	// added, so the backup can't simply be restored on top of what's there.
	if _, err := conn.ExecContext(ctx, "ALTER SCHEMA public RENAME TO "+restoreHoldingSchema+"; CREATE SCHEMA public;"); err != nil {
		return fmt.Errorf("setting aside existing database: %w", err)
	}

	restoreErr := runClientTool(env, f, nil, pgRestoreTool, "--no-owner", "--no-privileges", "--exit-on-error", "--single-transaction", "--dbname="+dbName)
	if restoreErr != nil {
		if _, err := conn.ExecContext(ctx, "DROP SCHEMA public CASCADE; ALTER SCHEMA "+restoreHoldingSchema+" RENAME TO public;"); err != nil {
			return errors.Join(restoreErr, fmt.Errorf("putting back existing database: %w", err))
		}
	} else if _, err := conn.ExecContext(ctx, "DROP SCHEMA "+restoreHoldingSchema+" CASCADE;"); err != nil {
		return fmt.Errorf("removing replaced database: %w", err)
	}

	if err := db.Open(ctx, db.dbPath); err != nil {
		return errors.Join(restoreErr, fmt.Errorf("reopening database: %w", err))
	}

	return restoreErr
}
