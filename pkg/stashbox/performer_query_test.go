package stashbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePerformerSearchStashBox answers SearchPerformer with one performer
// named after the search term, and records the terms it was asked for.
type fakePerformerSearchStashBox struct {
	mu    sync.Mutex
	terms []string
}

func (s *fakePerformerSearchStashBox) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	var req struct {
		OperationName string `json:"operationName"`
		Variables     struct {
			Term string `json:"term"`
		} `json:"variables"`
	}
	_ = json.Unmarshal(body, &req)

	if req.OperationName != "SearchPerformer" {
		http.Error(w, "unexpected operation", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.terms = append(s.terms, req.Variables.Term)
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	writeRaw(w, fmt.Sprintf(`{"data":{"searchPerformer":[{"id":%q,"name":%q,"aliases":[],"urls":[],"images":[],"tattoos":[],"piercings":[],"deleted":false}]}}`,
		"remote-"+req.Variables.Term, req.Variables.Term))
}

func queryPerformersAgainstFake(t *testing.T, names []string) ([][]*models.ScrapedPerformer, *fakePerformerSearchStashBox) {
	t.Helper()

	fake := &fakePerformerSearchStashBox{}
	server := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer server.Close()

	client := NewClient(models.StashBox{Endpoint: server.URL, MaxRequestsPerMinute: 60000})

	ret, err := client.QueryPerformers(context.Background(), names)
	require.NoError(t, err)
	return ret, fake
}

// Each named performer is searched for, in order, and gets its own results;
// a blank name is not searched for and gets none.
func TestQueryPerformers_SearchesNamedPerformers(t *testing.T) {
	ret, fake := queryPerformersAgainstFake(t, []string{"Alice", "", "Bob"})

	assert.Equal(t, []string{"Alice", "Bob"}, fake.terms, "terms searched")

	require.Len(t, ret, 3)

	require.Len(t, ret[0], 1)
	assert.Equal(t, "Alice", *ret[0][0].Name)
	assert.Equal(t, "remote-Alice", *ret[0][0].RemoteSiteID)

	assert.Empty(t, ret[1], "blank name has no results")

	require.Len(t, ret[2], 1)
	assert.Equal(t, "Bob", *ret[2][0].Name)
	assert.Equal(t, "remote-Bob", *ret[2][0].RemoteSiteID)
}

// Only blank names: nothing is sent and each gets no results.
func TestQueryPerformers_BlankNamesOnly(t *testing.T) {
	ret, fake := queryPerformersAgainstFake(t, []string{"", ""})

	assert.Empty(t, fake.terms, "terms searched")
	require.Len(t, ret, 2)
	assert.Empty(t, ret[0])
	assert.Empty(t, ret[1])
}

func TestQueryPerformers_NoNames(t *testing.T) {
	ret, fake := queryPerformersAgainstFake(t, nil)

	assert.Empty(t, fake.terms)
	assert.Empty(t, ret)
}
