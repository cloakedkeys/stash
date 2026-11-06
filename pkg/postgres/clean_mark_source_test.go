package postgres

import (
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The clean mark goes on a line of its own, so a file that ends in a comment
// without a newline still gets it.
func TestCleanMarkSourceAfterComment(t *testing.T) {
	src := cleanMarkSource{fakeSource{"CREATE TABLE x (id INTEGER); -- the end"}}
	r, _, err := src.ReadUp(7)
	require.NoError(t, err)
	defer r.Close()
	body, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "CREATE TABLE x (id INTEGER); -- the end\n;UPDATE schema_migrations SET dirty = false WHERE version = 7;\n", string(body))
}

type fakeSource struct {
	body string
}

func (fakeSource) Open(string) (source.Driver, error) { return nil, nil }
func (fakeSource) Close() error                       { return nil }
func (fakeSource) First() (uint, error)               { return 7, nil }
func (fakeSource) Prev(uint) (uint, error)            { return 0, fs.ErrNotExist }
func (fakeSource) Next(uint) (uint, error)            { return 0, fs.ErrNotExist }
func (s fakeSource) ReadUp(uint) (io.ReadCloser, string, error) {
	return io.NopCloser(strings.NewReader(s.body)), "fake", nil
}
func (fakeSource) ReadDown(uint) (io.ReadCloser, string, error) { return nil, "", fs.ErrNotExist }
