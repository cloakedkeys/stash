package manager

import (
	"context"
	"errors"
	"time"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/logger"
)

type statisticsRefresher interface {
	RefreshStatistics(ctx context.Context, changedSince time.Time) error
}

// refreshStatisticsAfterJobs refreshes the database statistics each time a task
// finishes (#2030). Scans, Identify, Auto Tag and batch tasks can change many
// rows, such as giving images studios, without changing how many rows there
// are. SQLite's automatic refresh only notices the number of rows changing, so
// without this the statistics go stale and some lists get slow.
//
// Tasks that finish together, such as the generate task queued for each
// uploaded file, share one refresh.
func refreshStatisticsAfterJobs(ctx context.Context, jobs *job.Manager, db statisticsRefresher) {
	sub := jobs.Subscribe(ctx)
	go func() {
		for {
			var since time.Time
			select {
			case j, ok := <-sub.RemovedJob:
				if !ok {
					return
				}
				since = earliestStart(since, j)
			case <-ctx.Done():
				return
			}

			// take any other tasks that have finished meanwhile
		drain:
			for {
				select {
				case j, ok := <-sub.RemovedJob:
					if !ok {
						break drain
					}
					since = earliestStart(since, j)
				default:
					break drain
				}
			}

			if !since.IsZero() {
				refreshStatistics(ctx, db, since)
			}
		}
	}()
}

// earliestStart returns the earlier of since and when j started. A task that
// was cancelled before it started changed nothing, so it's left out.
func earliestStart(since time.Time, j job.Job) time.Time {
	if j.StartTime == nil {
		return since
	}
	if since.IsZero() || j.StartTime.Before(since) {
		return *j.StartTime
	}
	return since
}

func refreshStatistics(ctx context.Context, db statisticsRefresher, since time.Time) {
	err := db.RefreshStatistics(ctx, since)
	var migrationNeeded *database.MigrationNeededError
	switch {
	case err == nil:
	case errors.Is(err, database.ErrDatabaseNotInitialized), errors.As(err, &migrationNeeded):
		// nothing to refresh until setup or the migration is done
	default:
		logger.Warnf("refreshing database statistics: %v", err)
	}
}
