package stashbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stashapp/stash/pkg/models"
)

// fakeFingerprintStashBox answers FindScenesBySceneFingerprints. For every
// scene it is asked about, it returns one match whose id is that scene's first
// fingerprint hash, so a test can tell which scene each result belongs to.
// shortBy drops that many lists from the end of the first response; extra
// appends that many unasked-for lists to it.
type fakeFingerprintStashBox struct {
	shortBy int
	extra   int

	mu       sync.Mutex
	requests []int
}

func (s *fakeFingerprintStashBox) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	var req struct {
		OperationName string `json:"operationName"`
		Variables     struct {
			Fingerprints [][]struct {
				Hash string `json:"hash"`
			} `json:"fingerprints"`
		} `json:"variables"`
	}
	_ = json.Unmarshal(body, &req)

	if req.OperationName != "FindScenesBySceneFingerprints" {
		http.Error(w, "unexpected operation", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	first := len(s.requests) == 0
	s.requests = append(s.requests, len(req.Variables.Fingerprints))
	s.mu.Unlock()

	var lists []string
	for _, fps := range req.Variables.Fingerprints {
		lists = append(lists, "["+fakeSceneJSON(fps[0].Hash)+"]")
	}

	if first {
		lists = lists[:max(0, len(lists)-s.shortBy)]
		for i := 0; i < s.extra; i++ {
			lists = append(lists, "["+fakeSceneJSON(fmt.Sprintf("extra-%d", i))+"]")
		}
	}

	w.Header().Set("Content-Type", "application/json")
	writeRaw(w, `{"data":{"findScenesBySceneFingerprints":[`+strings.Join(lists, ",")+`]}}`)
}

func fakeSceneJSON(id string) string {
	return fmt.Sprintf(`{"id":%q,"title":%q,"urls":[],"images":[],"studio":null,"tags":[],"performers":[],"fingerprints":[]}`, id, id)
}

func md5Fingerprints(hash string) models.Fingerprints {
	return models.Fingerprints{{Type: models.FingerprintTypeMD5, Fingerprint: hash}}
}

// findAll runs FindScenesByFingerprints against the fake, turning a panic into
// a test failure so one bad case doesn't stop the rest.
func findAll(t *testing.T, fake *fakeFingerprintStashBox, fps []models.Fingerprints) (ret [][]*models.ScrapedScene) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer server.Close()

	client := NewClient(models.StashBox{Endpoint: server.URL, MaxRequestsPerMinute: 60000})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("FindScenesByFingerprints panicked: %v", r)
		}
	}()

	ret, err := client.FindScenesByFingerprints(context.Background(), fps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return ret
}

// resultIDs gives the id of each scene's single match, or "" for no match.
func resultIDs(t *testing.T, ret [][]*models.ScrapedScene) []string {
	t.Helper()

	ids := make([]string, len(ret))
	for i, r := range ret {
		switch len(r) {
		case 0:
		case 1:
			ids[i] = *r[0].RemoteSiteID
		default:
			t.Fatalf("scene %d: expected at most one match, got %d", i, len(r))
		}
	}
	return ids
}

func assertIDs(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d results, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("result %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func numberedFingerprints(n int) ([]models.Fingerprints, []string) {
	fps := make([]models.Fingerprints, n)
	ids := make([]string, n)
	for i := range fps {
		ids[i] = fmt.Sprintf("h%d", i)
		fps[i] = md5Fingerprints(ids[i])
	}
	return fps, ids
}

// A stash-box that returns fewer result lists than it was asked for must not
// crash; the scenes it left out are treated as having no match.
func TestFindScenesByFingerprints_ShortResults(t *testing.T) {
	t.Run("single scene, empty response", func(t *testing.T) {
		fake := &fakeFingerprintStashBox{shortBy: 1}
		ret := findAll(t, fake, []models.Fingerprints{md5Fingerprints("h0")})
		assertIDs(t, resultIDs(t, ret), []string{""})
	})

	t.Run("single scene lookup used by identify", func(t *testing.T) {
		fake := &fakeFingerprintStashBox{shortBy: 1}
		server := httptest.NewServer(http.HandlerFunc(fake.handler))
		defer server.Close()

		client := NewClient(models.StashBox{Endpoint: server.URL, MaxRequestsPerMinute: 60000})

		var ret []*models.ScrapedScene
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("FindSceneByFingerprints panicked: %v", r)
				}
			}()
			ret, err = client.FindSceneByFingerprints(context.Background(), md5Fingerprints("h0"))
		}()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ret) != 0 {
			t.Errorf("expected no match, got %d results", len(ret))
		}
	})

	t.Run("three scenes, two left out", func(t *testing.T) {
		fake := &fakeFingerprintStashBox{shortBy: 2}
		fps, _ := numberedFingerprints(3)
		ret := findAll(t, fake, fps)
		assertIDs(t, resultIDs(t, ret), []string{"h0", "", ""})
	})

	t.Run("scene without fingerprints keeps its place", func(t *testing.T) {
		fake := &fakeFingerprintStashBox{shortBy: 1}
		fps := []models.Fingerprints{md5Fingerprints("h0"), nil, md5Fingerprints("h2")}
		ret := findAll(t, fake, fps)
		assertIDs(t, resultIDs(t, ret), []string{"h0", "", ""})
	})

	t.Run("short first request, second request still lines up", func(t *testing.T) {
		// 45 scenes go out as a request of 40 and a request of 5. The first
		// comes back one short; the second request's results must still land
		// on their own scenes.
		fake := &fakeFingerprintStashBox{shortBy: 1}
		fps, ids := numberedFingerprints(45)
		ret := findAll(t, fake, fps)

		want := append([]string{}, ids...)
		want[39] = ""
		assertIDs(t, resultIDs(t, ret), want)

		if got := fake.requests; len(got) != 2 || got[0] != 40 || got[1] != 5 {
			t.Errorf("expected requests of 40 and 5 scenes, got %v", got)
		}
	})
}

// When the stash-box returns the full list, every scene gets its own result,
// exactly as before.
func TestFindScenesByFingerprints_FullResults(t *testing.T) {
	t.Run("three scenes", func(t *testing.T) {
		fps, ids := numberedFingerprints(3)
		ret := findAll(t, &fakeFingerprintStashBox{}, fps)
		assertIDs(t, resultIDs(t, ret), ids)
	})

	t.Run("scene without fingerprints is not sent and has no match", func(t *testing.T) {
		fake := &fakeFingerprintStashBox{}
		fps := []models.Fingerprints{md5Fingerprints("h0"), nil, md5Fingerprints("h2")}
		ret := findAll(t, fake, fps)
		assertIDs(t, resultIDs(t, ret), []string{"h0", "", "h2"})

		if got := fake.requests; len(got) != 1 || got[0] != 2 {
			t.Errorf("expected one request of 2 scenes, got %v", got)
		}
	})

	t.Run("more than one request", func(t *testing.T) {
		fps, ids := numberedFingerprints(85)
		fake := &fakeFingerprintStashBox{}
		ret := findAll(t, fake, fps)
		assertIDs(t, resultIDs(t, ret), ids)

		if got := fake.requests; len(got) != 3 || got[0] != 40 || got[1] != 40 || got[2] != 5 {
			t.Errorf("expected requests of 40, 40 and 5 scenes, got %v", got)
		}
	})

	t.Run("no scenes with fingerprints sends nothing", func(t *testing.T) {
		fake := &fakeFingerprintStashBox{}
		ret := findAll(t, fake, []models.Fingerprints{nil, nil})
		assertIDs(t, resultIDs(t, ret), []string{"", ""})

		if len(fake.requests) != 0 {
			t.Errorf("expected no requests, got %v", fake.requests)
		}
	})
}

// A stash-box that returns more lists than asked for keeps the behaviour it
// had before: each scene takes the list at its own position and the extra
// lists are ignored.
func TestFindScenesByFingerprints_LongResults(t *testing.T) {
	fps, ids := numberedFingerprints(3)
	ret := findAll(t, &fakeFingerprintStashBox{extra: 2}, fps)
	assertIDs(t, resultIDs(t, ret), ids)
}
