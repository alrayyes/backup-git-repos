//go:build integration

package gitlab_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
	"github.com/stretchr/testify/require"
)

// newRecordedIssueServer replays JSON recorded from a real GitLab CE
// container -- team/active-repo's own issues and each one's notes -- the
// same recorded-fixture approach newRecordedServer (lister_test.go's own,
// in recorded_test.go) uses. notes_active_2.json (the closed issue's notes)
// carries only GitLab-generated system notes ("changed the description",
// "changed status to closed"), which is what proves IssueExporter filters
// them out rather than writing them as comments: a closed issue with a real
// person's comment nowhere in its notes is still expected to come back
// with an empty Comments list, not a two-entry one full of activity log
// noise.
func newRecordedIssueServer(t *testing.T) *httptest.Server {
	t.Helper()

	issues := readFixture(t, "issues_active.json")
	notes := readIssueNoteFixtures(t)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(r.URL.Path, "/notes") {
			withoutSuffix := strings.TrimSuffix(r.URL.Path, "/notes")
			iid := withoutSuffix[strings.LastIndex(withoutSuffix, "/")+1:]
			if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
				_, _ = w.Write([]byte("[]"))
				return
			}
			_, _ = w.Write(notes[iid])
			return
		}

		if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte("[]"))
			return
		}
		_, _ = w.Write(issues)
	}))
}

// readIssueNoteFixtures reads every testdata/notes_active_<iid>.json into a
// map keyed by the iid string in its filename, rather than a hardcoded pair
// of names -- TestUpdateFixtures (gitlab tag) writes one such file per issue
// the fixture actually seeded, under whatever iid GitLab assigned it, and
// this stays correct however many exist or whatever those iids turn out to
// be.
func readIssueNoteFixtures(t *testing.T) map[string][]byte {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(testdataDir, "notes_active_*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "no notes_active_*.json fixtures found")

	notes := make(map[string][]byte, len(matches))
	for _, m := range matches {
		iid := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "notes_active_"), ".json")
		data, err := os.ReadFile(m)
		require.NoError(t, err)
		notes[iid] = data
	}
	return notes
}

func TestRecordedIssueExporterContract(t *testing.T) {
	t.Parallel()

	srv := newRecordedIssueServer(t)
	defer srv.Close()

	backup.TestIssueExporter(t, func(*testing.T) backup.MetadataExporter {
		client, err := gitlab.New(srv.URL, "unused")
		require.NoError(t, err)
		return gitlab.NewIssueExporter(client)
	})
}
