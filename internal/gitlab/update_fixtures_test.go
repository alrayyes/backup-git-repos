//go:build integration && gitlab

package gitlab_test

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "refresh recorded gitlab api fixtures against a real container")

// TestUpdateFixtures refreshes testdata/*.json from a real container's
// actual API responses. It's the gitlab tag's nightly-only cost paying for
// something every pull request gets to use for free: run with
// `go test -tags='integration gitlab' -run TestUpdateFixtures -update`.
func TestUpdateFixtures(t *testing.T) {
	if !*update {
		t.Skip("run with -update to refresh recorded fixtures")
	}

	f := start(t)

	writeFixture(t, "projects_active.json", fetchRawProjects(t, f, false))
	writeFixture(t, "projects_archived.json", fetchRawProjects(t, f, true))
}

func fetchRawProjects(t *testing.T, f fixture, archived bool) []byte {
	t.Helper()

	url := fmt.Sprintf("%s/api/v4/projects?membership=true&per_page=100&page=1&archived=%t", f.BaseURL, archived)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("PRIVATE-TOKEN", f.Token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return data
}

func writeFixture(t *testing.T, name string, data []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(testdataDir, name), data, 0o644))
}
