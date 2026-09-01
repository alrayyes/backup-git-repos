package gitlab_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// releaseAssetContent is the body releaseServer serves for the one asset it
// carries -- a fixed value rather than backup.TestReleaseAssetContent, since
// this test drives ReleaseExporter directly against hand-built JSON, not
// through backup.TestReleaseExporter's own fixture-seeding contract.
const releaseAssetContent = "gitlab release asset payload"

// releaseServer replies to the releases list with two releases -- one
// carrying a single asset link whose direct_asset_url points back at this
// same server, one carrying none -- and serves that asset's raw content at
// a fixed path, gated on the PRIVATE-TOKEN header every authenticated
// request to this client carries. This is what gives ReleaseExporter
// fast-lane (go test ./..., no build tag) coverage: the
// integration-tagged contract test in recorded_release_test.go covers the
// same exporter against fixtures recorded from a real GitLab CE container,
// but Codecov's patch-coverage gate only sees what runs without that tag.
func releaseServer(t *testing.T) *httptest.Server {
	t.Helper()

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/asset-content") {
			assert.Equal(t, "unused", r.Header.Get("PRIVATE-TOKEN"))
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte(releaseAssetContent))

			return
		}

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte(`[]`))

			return
		}

		_, _ = fmt.Fprintf(w, `[
			{"tag_name":"v1.0.0","name":"v1.0.0","description":"release notes","author":{"username":"alice"},
			 "created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-02T00:00:00Z",
			 "assets":{"links":[{"name":"artifact.txt","url":%q,"direct_asset_url":%q}]}},
			{"tag_name":"v0.9.0","name":"v0.9.0","description":"no assets here","author":{"username":"alice"},
			 "created_at":"2025-12-01T00:00:00Z","released_at":"2025-12-02T00:00:00Z","assets":{"links":[]}}
		]`, srv.URL+"/asset-content", srv.URL+"/asset-content")
	}))

	return srv
}

func TestReleaseExporterUnit(t *testing.T) {
	t.Parallel()

	srv := releaseServer(t)
	defer srv.Close()

	client, err := gitlab.New(srv.URL, "unused")
	require.NoError(t, err)

	exp := gitlab.NewReleaseExporter(client)
	require.Equal(t, backup.MetadataReleases, exp.Kind())

	dir := t.TempDir()
	require.NoError(t, exp.Export(t.Context(), backup.Repo{Path: "team/repo"}, dir))

	data, err := os.ReadFile(filepath.Join(dir, "v1.0.0", "release.json"))
	require.NoError(t, err)

	var release backup.Release
	require.NoError(t, json.Unmarshal(data, &release))
	require.Equal(t, "alice", release.Author)
	require.Len(t, release.Assets, 1)
	require.Equal(t, "artifact.txt", release.Assets[0].Name)
	require.Equal(t, int64(len(releaseAssetContent)), release.Assets[0].Size,
		"gitlab's own API reports no asset size, so it must come from the actual bytes written")

	content, err := os.ReadFile(filepath.Join(dir, "v1.0.0", "assets", "artifact.txt"))
	require.NoError(t, err)
	require.Equal(t, releaseAssetContent, string(content))

	require.NoDirExists(t, filepath.Join(dir, "v0.9.0", "assets"),
		"a release with no uploaded assets must create no assets directory")
}
