package postgres

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogArgs(t *testing.T) {
	story := strings.Repeat("é", 300) // 600 bytes
	got := logArgs([]interface{}{1, "short", story, make([]byte, 500), nil})

	assert.Equal(t, 1, got[0])
	assert.Equal(t, "short", got[1])
	s := got[2].(string)
	assert.True(t, strings.HasPrefix(s, strings.Repeat("é", 100)), "starts with the first 200 bytes")
	assert.True(t, strings.HasSuffix(s, "… (600 bytes)"), s)
	assert.Less(t, len(s), 250)
	assert.Equal(t, "<500 bytes>", got[3])
	assert.Nil(t, got[4])
}
