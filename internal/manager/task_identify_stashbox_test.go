package manager

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stashapp/stash/pkg/match"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/stashbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedFingerprintGetter struct{}

func (fixedFingerprintGetter) GetScenesFingerprints(_ context.Context, ids []int) ([]models.Fingerprints, error) {
	ret := make([]models.Fingerprints, len(ids))
	for i := range ids {
		ret[i] = models.Fingerprints{{Type: models.FingerprintTypeMD5, Fingerprint: "abc123"}}
	}
	return ret, nil
}

// newIdentifyStashBox starts a stand-in stash-box that answers every
// fingerprint lookup with the given response body.
func newIdentifyStashBox(t *testing.T, body string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func newTestStashboxSource(endpoint string) stashboxSource {
	db := mocks.NewDatabase()
	return stashboxSource{
		Client:                 stashbox.NewClient(models.StashBox{Endpoint: endpoint, MaxRequestsPerMinute: 60000}),
		endpoint:               endpoint,
		txnManager:             db.Repository().TxnManager,
		sceneFingerprintGetter: fixedFingerprintGetter{},
		matcher:                match.SceneRelationships{},
	}
}

// A stash-box that answers with no result list for the scene must give
// identify "no match" for it rather than crash the job.
func TestStashboxSourceScrapeScenes_NoResultList(t *testing.T) {
	server := newIdentifyStashBox(t, `{"data":{"findScenesBySceneFingerprints":[]}}`)
	src := newTestStashboxSource(server.URL)

	var ret []*models.ScrapedScene
	var err error
	require.NotPanics(t, func() {
		ret, err = src.ScrapeScenes(context.Background(), 1)
	})
	require.NoError(t, err)
	assert.Empty(t, ret)
}

// With a result list for the scene, identify still gets the match.
func TestStashboxSourceScrapeScenes_Match(t *testing.T) {
	server := newIdentifyStashBox(t, `{"data":{"findScenesBySceneFingerprints":[[`+
		`{"id":"remote-1","title":"Remote","urls":[],"images":[],"studio":null,"tags":[],"performers":[],"fingerprints":[]}`+
		`]]}}`)
	src := newTestStashboxSource(server.URL)

	ret, err := src.ScrapeScenes(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, ret, 1)
	require.NotNil(t, ret[0].RemoteSiteID)
	assert.Equal(t, "remote-1", *ret[0].RemoteSiteID)
}
