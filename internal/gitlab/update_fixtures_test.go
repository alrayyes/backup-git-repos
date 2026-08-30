//go:build integration && gitlab

package gitlab_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "refresh recorded gitlab api fixtures against a real container")

// fixtureRoles maps the "role" a recorded fixture's filename carries --
// active, archived, empty -- to the seeded project it was captured
// against. wikis_active.json and snippets_active.json are the only pair
// of these six actually seeded with content; the rest exist to prove the
// "no empty entry" side of issue #6 stays recorded too.
var fixtureRoles = map[string]string{
	"active":   backup.TestActiveRepoPath,
	"archived": backup.TestArchivedRepoPath,
	"empty":    backup.TestEmptyRepoPath,
}

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

	for role, path := range fixtureRoles {
		writeFixture(t, "wikis_"+role+".json", fetchRawProjectSub(t, f, path, "wikis"))
		writeFixture(t, "snippets_"+role+".json", fetchRawProjectSub(t, f, path, "snippets"))
	}

	updateIssueFixtures(t, f)
}

// updateIssueFixtures refreshes issues_active.json and one
// notes_active_<iid>.json per issue seed.seedIssues creates on
// team/active-repo -- the two backup.TestIssueExporter itself expects, an
// open one and a closed one -- from the real iids GitLab assigned them
// rather than the 1/2 this package's own hand-authored testdata assumed
// before the first real -update run recorded actual values.
func updateIssueFixtures(t *testing.T, f fixture) {
	t.Helper()

	issuesRaw := fetchRawProjectSub(t, f, backup.TestActiveRepoPath, "issues")
	writeFixture(t, "issues_active.json", issuesRaw)

	var issues []struct {
		IID int `json:"iid"`
	}
	require.NoError(t, json.Unmarshal(issuesRaw, &issues))

	for _, issue := range issues {
		sub := "issues/" + strconv.Itoa(issue.IID) + "/notes"
		notesRaw := fetchRawProjectSub(t, f, backup.TestActiveRepoPath, sub)
		writeFixture(t, "notes_active_"+strconv.Itoa(issue.IID)+".json", notesRaw)
	}
}

func fetchRawProjects(t *testing.T, f fixture, archived bool) []byte {
	t.Helper()

	u := fmt.Sprintf("%s/api/v4/projects?membership=true&per_page=100&page=1&archived=%t", f.BaseURL, archived)
	return fetchRaw(t, f, u)
}

// fetchRawProjectSub fetches a per-project sub-resource -- wikis,
// snippets -- addressed by the project's own namespaced path, the same
// form the adapter itself queries in gitlab.go.
func fetchRawProjectSub(t *testing.T, f fixture, projectPath, sub string) []byte {
	t.Helper()

	u := fmt.Sprintf("%s/api/v4/projects/%s/%s", f.BaseURL, url.PathEscape(projectPath), sub)
	return fetchRaw(t, f, u)
}

func fetchRaw(t *testing.T, f fixture, rawURL string) []byte {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
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
