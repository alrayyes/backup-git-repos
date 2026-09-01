//go:build integration && gitlab

package gitlab_test

import (
	"bytes"
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
// No t.Parallel() here: it writes testdata/*.json files shared with every
// other test in the package, a real exception rather than left off by
// default.
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
	updateReleaseFixtures(t, f)
}

// updateReleaseFixtures refreshes testdata/releases_active.json and
// testdata/release_asset_artifact.txt from a real container's actual API
// responses. The recorded release's own asset link URL points at the
// container's own dynamic base URL, which changes every recording session,
// so it's substituted here for releaseBaseURLPlaceholder --
// newRecordedReleaseServer (recorded_release_test.go, integration tag
// only) substitutes it back for whatever httptest server it's replaying
// against.
func updateReleaseFixtures(t *testing.T, f fixture) {
	t.Helper()

	raw := fetchRawProjectSub(t, f, backup.TestActiveRepoPath, "releases")

	var releases []struct {
		Assets struct {
			Links []struct {
				URL string `json:"url"`
			} `json:"links"`
		} `json:"assets"`
	}
	require.NoError(t, json.Unmarshal(raw, &releases))

	var assetURL string
	for _, r := range releases {
		if len(r.Assets.Links) > 0 {
			assetURL = r.Assets.Links[0].URL

			break
		}
	}
	require.NotEmpty(t, assetURL, "expected at least one recorded release to carry an uploaded asset link")

	// GitLab reports the link against its own configured external_url,
	// which inside a container is not the host-mapped address this test
	// process actually reaches it on -- the same problem
	// ReleaseExporter.resolveAssetURL already solves in the production
	// code by rebuilding the link against the client's own known-good
	// base URL, keeping only the path and query. Skipping this here would
	// fetch whatever (if anything) happens to answer on the container's
	// self-reported host and port from this machine instead of the
	// container under test.
	parsed, err := url.Parse(assetURL)
	require.NoError(t, err)
	resolved := f.BaseURL + parsed.Path
	if parsed.RawQuery != "" {
		resolved += "?" + parsed.RawQuery
	}

	writeFixture(t, "release_asset_artifact.txt", fetchRaw(t, f, resolved))
	writeFixture(t, "releases_active.json", bytes.ReplaceAll(raw, []byte(f.BaseURL), []byte(releaseBaseURLPlaceholder)))
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
