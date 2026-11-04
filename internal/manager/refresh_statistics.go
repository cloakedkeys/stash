package manager

import (
	"context"
	"errors"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/logger"
)

type statisticsRefresher interface {
	RefreshStatistics(ctx context.Context) error
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
			ran := false
			select {
			case j, ok := <-sub.RemovedJob:
				if !ok {
					return
				}
				ran = started(j)
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
					ran = ran || started(j)
				default:
					break drain
				}
			}

			if ran {
				refreshStatistics(ctx, db)
			}
		}
	}()
}

// started reports whether j ran. A task cancelled before it started changed
// nothing.
func started(j job.Job) bool {
	return j.StartTime != nil
}

func refreshStatistics(ctx context.Context, db statisticsRefresher) {
	err := db.RefreshStatistics(ctx)
	var migrationNeeded *database.MigrationNeededError
	switch {
	case err == nil:
	case errors.Is(err, database.ErrDatabaseNotInitialized), errors.As(err, &migrationNeeded):
		// nothing to refresh until setup or the migration is done
	default:
		logger.Warnf("refreshing database statistics: %v", err)
	}
}
