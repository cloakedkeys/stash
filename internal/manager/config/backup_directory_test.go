package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBackupDirectoryPathOrDefault(t *testing.T) {
	configFile := filepath.Join("stash", "config.yml")

	tests := []struct {
		name     string
		database string
		backup   string
		want     string
	}{
		{"file database uses its directory", filepath.Join("data", "stash.sqlite"), "", "data"},
		{"connection string uses the config directory", "postgresql://user:secret@host:5432/stash?sslmode=disable", "", "stash"},
		{"configured directory wins", "postgresql://user:secret@host:5432/stash", "backups", "backups"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := InitializeEmpty()
			c.SetConfigFile(configFile)
			c.SetString(Database, tt.database)
			c.SetString(BackupDirectoryPath, tt.backup)

			assert.Equal(t, tt.want, c.GetBackupDirectoryPathOrDefault())
		})
	}
}
