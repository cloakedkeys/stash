package api

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stretchr/testify/assert"
)

func TestCurrentUserIsAdmin(t *testing.T) {
	ctx := context.Background()
	assert.True(t, currentUserIsAdmin(ctx), "no credentials required")

	admin := session.SetCurrentUser(ctx, models.User{Roles: models.Roles{models.RoleEnumAdmin}})
	assert.True(t, currentUserIsAdmin(admin))

	modify := session.SetCurrentUser(ctx, models.User{Roles: models.Roles{models.RoleEnumModify}})
	assert.False(t, currentUserIsAdmin(modify))

	read := session.SetCurrentUser(ctx, models.User{Roles: models.Roles{models.RoleEnumRead}})
	assert.False(t, currentUserIsAdmin(read))
}

func TestRedactConfig(t *testing.T) {
	logFile := "/secret/stash.log"
	location := "/secret/performers"
	handyKey := "secret-handy-key"
	cdpPath := "ws://secret/"
	box := &models.StashBox{Endpoint: "https://stashbox.example.com", APIKey: "secret-api-key", Name: "box"}

	c := &ConfigResult{
		General: &ConfigGeneralResult{
			DatabasePath:                 "/secret/db",
			BackupDirectoryPath:          "/secret/backup",
			DeleteTrashPath:              "/secret/trash",
			GeneratedPath:                "/secret/generated",
			MetadataPath:                 "/secret/metadata",
			ConfigFilePath:               "/secret/config",
			ScrapersPath:                 "/secret/scrapers",
			PluginsPath:                  "/secret/plugins",
			CachePath:                    "/secret/cache",
			BlobsPath:                    "/secret/blobs",
			FfmpegPath:                   "/secret/ffmpeg",
			FfprobePath:                  "/secret/ffprobe",
			PythonPath:                   "/secret/python",
			LogFile:                      &logFile,
			CustomPerformerImageLocation: &location,
			StashBoxes:                   []*models.StashBox{box},
			ParallelTasks:                3,
		},
		Interface: &ConfigInterfaceResult{HandyKey: &handyKey},
		Scraping:  &ConfigScrapingResult{ScraperCDPPath: &cdpPath},
		Dlna:      &ConfigDLNAResult{WhitelistedIPs: []string{"10.0.0.1"}},
	}

	redactConfig(c)

	g := c.General
	for _, path := range []string{
		g.DatabasePath, g.BackupDirectoryPath, g.DeleteTrashPath, g.GeneratedPath, g.MetadataPath,
		g.ConfigFilePath, g.ScrapersPath, g.PluginsPath, g.CachePath, g.BlobsPath, g.FfmpegPath,
		g.FfprobePath, g.PythonPath,
	} {
		assert.Empty(t, path)
	}
	assert.Nil(t, g.LogFile)
	assert.Nil(t, g.CustomPerformerImageLocation)
	assert.Empty(t, *c.Interface.HandyKey)
	assert.Empty(t, *c.Scraping.ScraperCDPPath)
	assert.Empty(t, c.Dlna.WhitelistedIPs)

	// the interface still lists the stash-boxes and settings
	assert.Equal(t, "https://stashbox.example.com", g.StashBoxes[0].Endpoint)
	assert.Empty(t, g.StashBoxes[0].APIKey)
	assert.Equal(t, 3, g.ParallelTasks)

	// the stored settings aren't changed
	assert.Equal(t, "secret-api-key", box.APIKey)
}
