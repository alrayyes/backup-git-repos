//go:build integration

package github_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/github"
	"github.com/stretchr/testify/require"
)

// releaseFixtureServer replays testdata/releases.json -- GitHub.com has no
// self-hostable instance, so this is the only fixture ReleaseExporter will
// ever have, the same reasoning TestRecordedListerContract's own
// testdata/repos.json already follows. The fixture's one uploaded asset
// names its download URL with the literal placeholder
// "BASE_URL_PLACEHOLDER", substituted here for the httptest server's own
// URL: unlike an issue's comments (fetched by a URL this adapter itself
// builds from the repository path), a release asset's download address is
// data the API response carries, so the fixture has to name where its
// content lives.
func releaseFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()

	releases := readFixture(t, "releases.json")
	assetContent := readFixture(t, "release_asset_artifact.txt")

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/release-assets/artifact.txt") {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(assetContent)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte("[]"))

			return
		}
		_, _ = w.Write(bytes.ReplaceAll(releases, []byte("BASE_URL_PLACEHOLDER"), []byte(srv.URL)))
	}))

	return srv
}

func TestRecordedReleaseExporterContract(t *testing.T) {
	srv := releaseFixtureServer(t)
	defer srv.Close()

	backup.TestReleaseExporter(t, func(*testing.T) backup.MetadataExporter {
		client, err := github.New(srv.URL, "unused")
		require.NoError(t, err)

		return github.NewReleaseExporter(client)
	})
}
