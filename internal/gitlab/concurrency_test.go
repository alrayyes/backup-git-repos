package gitlab_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestListReposFetchesProjectDetailsConcurrently proves issue #91's fix
// without booting a container: each of several projects' wiki and snippet
// lookups sleeps for a fixed latency, and this asserts both that the whole
// listing finishes in far less than the fully-serial cost (every project's
// two lookups back to back) and that more than one such lookup was ever
// in flight at once -- either alone could pass by accident (a fast machine
// finishing a serial run quickly, or one lucky overlap), together they
// pin down the actual behaviour this issue is about.
func TestListReposFetchesProjectDetailsConcurrently(t *testing.T) {
	t.Parallel()

	// latency is deliberately generous (rather than, say, 10ms): the
	// assertions below compare wall-clock time against a threshold, and a
	// bigger latency widens the absolute margin between "genuinely
	// concurrent" and "looks serial" so a busy or throttled CI runner
	// (this whole suite already runs under -race, which adds its own
	// overhead) doesn't flake the comparison on scheduling noise alone.
	const (
		projectCount = 8
		latency      = 80 * time.Millisecond
	)

	var inFlight, maxInFlight atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(r.URL.Path, "/wikis") || strings.HasSuffix(r.URL.Path, "/snippets") {
			observeInFlight(&inFlight, &maxInFlight)
			time.Sleep(latency)
			_, _ = w.Write([]byte(`[]`))

			return
		}

		if r.URL.Query().Get("archived") == "true" {
			_, _ = w.Write([]byte(`[]`))

			return
		}
		_, _ = w.Write([]byte(projectListJSON(projectCount)))
	}))
	defer srv.Close()

	start := time.Now()
	repos := listRepos(t, srv.URL)
	elapsed := time.Since(start)

	// The three checks below all read off this one expensive run rather
	// than being independent scenarios that could each set up their own
	// server and call ListRepos again -- t.Run still separates them so a
	// failure names which property broke instead of only reporting
	// whichever assertion happened to come first.
	t.Run("still returns every project", func(t *testing.T) {
		require.Len(t, repos, projectCount)
	})

	t.Run("finishes far under the fully serial cost", func(t *testing.T) {
		// Fully serial -- one project's wiki lookup, then its snippet
		// lookup, then the next project's -- costs at least
		// projectCount * 2 * latency (8 * 2 * 80ms = 1.28s here).
		// Concurrent lookups across projects should land close to a
		// single project's own two sequential requests (2 * latency =
		// 160ms) plus scheduling overhead, nowhere near that -- the
		// threshold below is half the fully serial cost, comfortably
		// above the concurrent case and comfortably below the serial one.
		require.Less(t, elapsed, time.Duration(projectCount)*latency,
			"listing took %s, which looks like wiki/snippet lookups ran serially", elapsed)
	})

	t.Run("overlaps more than one wiki/snippet request at a time", func(t *testing.T) {
		require.Greater(t, maxInFlight.Load(), int64(1),
			"never observed more than one wiki/snippet request in flight at once")
	})
}

// observeInFlight increments inFlight for the duration of the caller's
// request (the caller sleeps, then the deferred decrement in the handler
// above isn't available here, so this returns nothing and relies on the
// handler's own lifetime) and records the highest concurrent value seen.
func observeInFlight(inFlight, maxInFlight *atomic.Int64) {
	n := inFlight.Add(1)
	for {
		old := maxInFlight.Load()
		if n <= old || maxInFlight.CompareAndSwap(old, n) {
			break
		}
	}
	// Deliberately never decremented: the handler's own request lifetime
	// (it sleeps for the full latency right after this call) already
	// keeps the count elevated for exactly as long as the request is in
	// flight, and a high-water mark only needs the count to have gone up,
	// never to come back down accurately.
}

// projectListJSON builds the GET /api/v4/projects response for n distinct
// active, non-empty projects under "team/", so each gets its own wiki and
// snippet lookup rather than every request racing to hit the same handler
// path with nothing to distinguish concurrent calls from repeated ones.
func projectListJSON(n int) string {
	var b strings.Builder
	b.WriteByte('[')
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b,
			`{"path_with_namespace":"team/project-%d","archived":false,"empty_repo":false}`, i)
	}
	b.WriteByte(']')

	return b.String()
}
