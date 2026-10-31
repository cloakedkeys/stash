package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A build with no update repository must not ask GitHub about any other repository.
func TestGetLatestReleaseWithoutUpdateRepo(t *testing.T) {
	release, err := GetLatestRelease(context.Background())
	assert.Nil(t, release)
	assert.ErrorIs(t, err, errNoUpdateCheck)
}
