//go:build integration

package gitlab_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
	"github.com/stretchr/testify/require"
)

// releaseBaseURLPlaceholder is what TestUpdateFixtures (gitlab tag)
// substitutes for a real container's own dynamic base URL when it records
// testdata/releases_active.json -- see that function's own doc comment.
// newRecordedReleaseServer substitutes it back for its own URL before
// replaying the fixture.
const releaseBaseURLPlaceholder = "GITLAB_BASE_URL_PLACEHOLDER"

// newRecordedReleaseServer replays JSON recorded from a real GitLab CE
// container -- team/active-repo's own releases -- the same recorded-fixture
// approach newRecordedServer (lister_test.go's own, in recorded_test.go)
// uses. The recorded release's own asset link now points back at this same
// server once the placeholder substitution below runs, and its content --
// also recorded, in release_asset_artifact.txt -- is served at whatever
// path GitLab's own file-upload endpoint gave it when the fixture was
// recorded.
func newRecordedReleaseServer(t *testing.T) *httptest.Server {
	t.Helper()

	releases := readFixture(t, "releases_active.json")
	assetContent := readFixture(t, "release_asset_artifact.txt")

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/uploads/") {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(assetContent)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte(`[]`))

			return
		}
		_, _ = w.Write(bytes.ReplaceAll(releases, []byte(releaseBaseURLPlaceholder), []byte(srv.URL)))
	}))

	return srv
}

func TestRecordedReleaseExporterContract(t *testing.T) {
	t.Parallel()

	srv := newRecordedReleaseServer(t)
	defer srv.Close()

	backup.TestReleaseExporter(t, func(*testing.T) backup.MetadataExporter {
		client, err := gitlab.New(srv.URL, "unused")
		require.NoError(t, err)

		return gitlab.NewReleaseExporter(client)
	})
}
