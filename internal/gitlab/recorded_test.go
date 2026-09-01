//go:build integration

package gitlab_test

import (
	"context"
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
	"github.com/stretchr/testify/require"
)

const testdataDir = "testdata"

// newRecordedServer replays the JSON recorded from a real GitLab CE
// container -- projects, and each seeded project's wikis and snippets --
// rather than booting one. GitLab CE takes several minutes to boot, too
// long for every pull request, so this is what gives the adapter coverage
// on every one of them; refresh the recording with
// `go test -tags='integration gitlab' -run TestUpdateFixtures -update`.
func newRecordedServer(t *testing.T) *httptest.Server {
	t.Helper()

	active := readFixture(t, "projects_active.json")
	archived := readFixture(t, "projects_archived.json")
	wikis := map[string][]byte{
		"active":   readFixture(t, "wikis_active.json"),
		"archived": readFixture(t, "wikis_archived.json"),
		"empty":    readFixture(t, "wikis_empty.json"),
	}
	snippets := map[string][]byte{
		"active":   readFixture(t, "snippets_active.json"),
		"archived": readFixture(t, "snippets_archived.json"),
		"empty":    readFixture(t, "snippets_empty.json"),
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if role, ok := projectRole(r.URL.Path, "/wikis"); ok {
			_, _ = w.Write(wikis[role])
			return
		}
		if role, ok := projectRole(r.URL.Path, "/snippets"); ok {
			_, _ = w.Write(snippets[role])
			return
		}

		if r.URL.Query().Get("archived") == "true" {
			_, _ = w.Write(archived)
			return
		}
		_, _ = w.Write(active)
	}))
}

// projectRole extracts which seeded project (active, archived, empty) a
// per-project sub-resource request -- .../projects/<path>/wikis or
// .../projects/<path>/snippets -- names, from the request's URL path.
// net/http already decodes a %2F in the project's own path segment back
// into a literal "/" before this ever sees it, so the project's path
// reads out as an ordinary substring rather than one encoded segment.
func projectRole(path, suffix string) (string, bool) {
	if !strings.HasSuffix(path, suffix) {
		return "", false
	}

	projectPath := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v4/projects/"), suffix)
	switch projectPath {
	case backup.TestActiveRepoPath:
		return "active", true
	case backup.TestArchivedRepoPath:
		return "archived", true
	case backup.TestEmptyRepoPath:
		return "empty", true
	default:
		return "", false
	}
}

// TestRecordedListerContract runs the same contract suite the real GitLab
// container satisfies (lister_test.go, gitlab tag), but against JSON
// recorded from a real container rather than booting one.
func TestRecordedListerContract(t *testing.T) {
	t.Parallel()

	srv := newRecordedServer(t)
	defer srv.Close()

	backup.TestLister(t, func(*testing.T) backup.Lister {
		client, err := gitlab.New(srv.URL, "unused")
		require.NoError(t, err)
		return client
	})
}

// TestRecordedListerIncludesWikisAndSnippets asserts the issue #6
// acceptance criteria the generic backup.TestLister contract above doesn't
// know to check, against the same recorded fixtures: a project's wiki and
// snippet are listed under their own paths, and a project with neither
// gets no entry for either.
func TestRecordedListerIncludesWikisAndSnippets(t *testing.T) {
	t.Parallel()

	srv := newRecordedServer(t)
	defer srv.Close()

	client, err := gitlab.New(srv.URL, "unused")
	require.NoError(t, err)

	repos, err := client.ListRepos(context.Background(), backup.StateAll)
	require.NoError(t, err)

	paths := make([]string, len(repos))
	for i, r := range repos {
		paths[i] = r.Path
	}

	t.Run("includes the project wiki", func(t *testing.T) {
		t.Parallel()
		require.Contains(t, paths, backup.TestActiveRepoPath+".wiki")
	})

	t.Run("includes the project snippet under a distinct path", func(t *testing.T) {
		t.Parallel()
		snippetPath := recordedSnippetPath(t)
		require.NotEqual(t, backup.TestActiveRepoPath, snippetPath)
		require.Contains(t, paths, snippetPath)
	})

	t.Run("creates no wiki or snippet entries for a project with neither", func(t *testing.T) {
		t.Parallel()
		require.NotContains(t, paths, backup.TestArchivedRepoPath+".wiki")
		require.NotContains(t, paths, backup.TestEmptyRepoPath+".wiki")
	})
}

// recordedSnippetPath computes the path the fixture's own snippet is
// expected under, from the ID recorded in snippets_active.json rather
// than a hardcoded value -- the same ID a live container assigned it,
// which -update has no control over.
func recordedSnippetPath(t *testing.T) string {
	t.Helper()

	var snippets []struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.Unmarshal(readFixture(t, "snippets_active.json"), &snippets))
	require.NotEmpty(t, snippets)

	return fmt.Sprintf("%s/snippets/%d", backup.TestActiveRepoPath, snippets[0].ID)
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir, name))
	require.NoError(t, err)
	return data
}
