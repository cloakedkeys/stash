//go:generate go run github.com/99designs/gqlgen
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"runtime/pprof"

	// fallback timezone database for systems without tzdata installed
	_ "time/tzdata"

	"github.com/spf13/pflag"

	"github.com/stashapp/stash/internal/api"
	"github.com/stashapp/stash/internal/build"
	"github.com/stashapp/stash/internal/desktop"
	"github.com/stashapp/stash/internal/log"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/internal/stoprequests"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/postgres"
	"github.com/stashapp/stash/ui"

	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

var exitCode = 0

func main() {
	// Stop requests are caught from before main starts (see stoprequests), so
	// that one that comes during startup doesn't end Stash at once, perhaps in
	// the middle of creating the database (#2103). stopCtx is cancelled once a
	// stop request takes effect.
	stopCtx, cancelStopCtx := contextUntil(stoprequests.Stopped())
	defer cancelStopCtx()

	defer func() {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	defer recoverPanic()

	initLogTemp()

	helpFlag := false
	pflag.BoolVarP(&helpFlag, "help", "h", false, "show this help text and exit")

	versionFlag := false
	pflag.BoolVarP(&versionFlag, "version", "v", false, "show version number and exit")

	cpuProfilePath := ""
	pflag.StringVar(&cpuProfilePath, "cpuprofile", "", "write cpu profile to file")

	getUsersFlag := false
	pflag.BoolVar(&getUsersFlag, "users", false, "print all user names and exit")

	resetPasswordFlag := ""
	pflag.StringVar(&resetPasswordFlag, "reset-password", "", "reset the password for the specified user and exit")

	migrateSQLiteFlag := ""
	pflag.StringVar(&migrateSQLiteFlag, "migrate-sqlite-to-postgres", "", "copy this SQLite file into the empty PostgreSQL database set in STASH_DATABASE and exit")

	pflag.Parse()

	if helpFlag {
		pflag.Usage()
		return
	}

	if versionFlag {
		fmt.Println(build.VersionString())
		return
	}

	cfg, err := config.Initialize()
	if err != nil {
		exitError(fmt.Errorf("config initialization error: %w", err))
		return
	}

	l := initLog(cfg)

	stoprequests.OnWaiting(func() {
		logger.Info("Stash will stop once the database upgrade has finished")
	})

	if cpuProfilePath != "" {
		f, err := initProfiling(cpuProfilePath)
		if err != nil {
			exitError(err)
			return
		}
		defer f.Close()
		defer pprof.StopCPUProfile()
	}

	if migrateSQLiteFlag != "" {
		if stoppedDuringStartup(stoprequests.Requested(), nil) {
			return
		}

		// before the manager, which would open the database. A stop waits for
		// the PostgreSQL schema to be built, then ends the copy, which runs in
		// a single transaction, so nothing is left half copied.
		err := migrateSQLiteToPostgres(stopCtx, cfg, migrateSQLiteFlag)
		finishCopy(stoprequests.Requested(), err)
		return
	}

	// initialise desktop.IsDesktop here so that it doesn't get affected by
	// ffmpeg hardware checks later on
	desktop.InitIsDesktop()

	if stoppedDuringStartup(stoprequests.Requested(), nil) {
		return
	}

	// A stop request cancels opening the database, such as while waiting for
	// a PostgreSQL server, but waits for a schema build or upgrade to finish.
	mgr, err := manager.Initialize(stopCtx, cfg, l)
	if stoppedDuringStartup(stoprequests.Requested(), err) {
		if mgr != nil {
			mgr.Shutdown()
		}
		return
	}
	if err != nil {
		exitError(fmt.Errorf("manager initialization error: %w", err))
		return
	}
	defer mgr.Shutdown()

	if getUsersFlag || resetPasswordFlag != "" {
		// as before #2103, a stop ends these commands at once
		stoprequests.Release()
	}

	if getUsersFlag {
		if err := mgr.PrintUsers(context.Background()); err != nil {
			exitError(fmt.Errorf("error printing users: %w", err))
		}
		return
	}

	if resetPasswordFlag != "" {
		if err := mgr.ResetUserPassword(context.Background(), resetPasswordFlag); err != nil {
			exitError(fmt.Errorf("error resetting user password: %w", err))
		}
		return
	}

	server, err := api.Initialize()
	if err != nil {
		exitError(fmt.Errorf("api initialization error: %w", err))
		return
	}
	defer server.Shutdown()

	exit := make(chan int)

	go func() {
		err := server.Start()
		if !errors.Is(err, http.ErrServerClosed) {
			exitError(fmt.Errorf("http server error: %w", err))
			exit <- 1
		}
	}()

	go stoprequests.Forward(exit)
	desktop.Start(exit, &ui.FaviconProvider)

	exitCode = <-exit
}

func migrateSQLiteToPostgres(ctx context.Context, cfg *config.Config, sqlitePath string) error {
	target := cfg.GetDatabasePath()
	if !database.IsPostgresPath(target) {
		return errors.New("set the STASH_DATABASE environment variable to the address of the PostgreSQL database to copy into")
	}
	return postgres.MigrateFromSQLite(ctx, sqlitePath, target, os.Stdout)
}

// finishCopy ends the copy to PostgreSQL, which returned err. If a stop
// request took effect and ended the copy, nothing was copied, and the exit
// code isn't 0, as when a stop ended the copy before #2103.
func finishCopy(stopRequested bool, err error) {
	switch {
	case err == nil:
	case stopRequested:
		exitCode = 1
		logger.Infof("Stopped before the copy to PostgreSQL finished. Nothing was copied, so it can be run again: %v", err)
	default:
		exitError(fmt.Errorf("error migrating to PostgreSQL: %w", err))
	}
}

// contextUntil returns a context that is cancelled once done is closed.
func contextUntil(done <-chan struct{}) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-done:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// stoppedDuringStartup reports whether a stop request has taken effect during
// startup, in which case Stash stops with the clean shutdown and exit code 0
// of a normal stop. err is the error of the startup step the stop ended, if
// any.
func stoppedDuringStartup(stopRequested bool, err error) bool {
	if !stopRequested {
		return false
	}
	logStopDuringStartup()
	if err != nil {
		logger.Debugf("Startup ended by the stop request: %v", err)
	}
	return true
}

// initLogTemp initializes a temporary logger for use before the config is loaded.
// Logs only error level message to stderr.
func initLogTemp() *log.Logger {
	l := log.NewLogger()
	l.Init("", true, "Error", 0)
	logger.Logger = l

	return l
}

func initLog(cfg *config.Config) *log.Logger {
	l := log.NewLogger()
	l.Init(cfg.GetLogFile(), cfg.GetLogOut(), cfg.GetLogLevel(), cfg.GetLogFileMaxSize())
	logger.Logger = l

	return l
}

func initProfiling(path string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("unable to create CPU profile file: %v", err)
	}

	if err = pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("could not start CPU profiling: %v", err)
	}

	logger.Infof("profiling to %s", path)

	return f, nil
}

func recoverPanic() {
	if err := recover(); err != nil {
		exitCode = 1
		logger.Errorf("panic: %v\n%s", err, debug.Stack())
		if desktop.IsDesktop() {
			desktop.FatalError(fmt.Errorf("panic: %v", err))
		}
	}
}

func exitError(err error) {
	exitCode = 1
	logger.Error(err)
	// #5784 - log to stdout as well as the logger
	// this does mean that it will log twice if the logger is set to stdout
	fmt.Println(err)
	if desktop.IsDesktop() {
		desktop.FatalError(err)
	}
}

func logStopDuringStartup() {
	logger.Info("Stopping, because a stop was requested while Stash was starting")
}
