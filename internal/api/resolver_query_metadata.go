package api

import (
	"context"

	"github.com/stashapp/stash/internal/manager"
)

func (r *queryResolver) SystemStatus(ctx context.Context) (*manager.SystemStatus, error) {
	status := manager.GetInstance().GetSystemStatus()

	if !currentUserIsAdmin(ctx) {
		status.DatabasePath = nil
		status.ConfigPath = nil
		status.WorkingDir = ""
		status.HomeDir = ""
		status.FfmpegPath = nil
		status.FfprobePath = nil
		status.LocalIPs = nil
	}

	return status, nil
}
