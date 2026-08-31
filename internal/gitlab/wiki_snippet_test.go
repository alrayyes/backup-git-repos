package gitlab_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
	"github.com/stretchr/testify/require"
)

// The tests below cover the wiki and snippet discovery logic against a
// fake HTTP server rather than a container -- no network, no Docker, part
// of the fast `go test ./...` lane -- since internal/gitlab otherwise has
// no test file that runs there at all (every other one in this package
// needs the integration or gitlab build tag), leaving this logic with no
// coverage from the lane codecov actually measures. Each scenario is its
// own top-level test, rather than one long TestListReposWikisAndSnippets
// of t.Run subtests, so a failure names the scenario directly and no
// single function grows past a length worth reading in one glance.

func TestWikiAndSnippetIncludedUnderTheirOwnPaths(t *testing.T) {
	t.Parallel()
	srv := fakeGitLab(t, map[string]handler{
		"/wikis":    jsonResponse(`[{"slug":"home"}]`),
		"/snippets": jsonResponse(`[{"id":7}]`),
	})
	defer srv.Close()

	repos := listRepos(t, srv.URL)

	require.Contains(t, paths(repos), "team/project.wiki")
	require.Contains(t, paths(repos), "team/project/snippets/7")
}

func TestNoWikiEntryForAProjectWithNoWikiContent(t *testing.T) {
	t.Parallel()
	srv := fakeGitLab(t, map[string]handler{
		"/wikis":    jsonResponse(`[]`),
		"/snippets": jsonResponse(`[]`),
	})
	defer srv.Close()

	repos := listRepos(t, srv.URL)

	require.NotContains(t, paths(repos), "team/project.wiki")
}

func TestNoSnippetEntriesForAProjectWithNoSnippets(t *testing.T) {
	t.Parallel()
	srv := fakeGitLab(t, map[string]handler{
		"/wikis":    jsonResponse(`[]`),
		"/snippets": jsonResponse(`[]`),
	})
	defer srv.Close()

	repos := listRepos(t, srv.URL)

	for _, p := range paths(repos) {
		require.NotContains(t, p, "/snippets/")
	}
}

func TestForbiddenWikiTreatedAsNoWikiContent(t *testing.T) {
	t.Parallel()
	srv := fakeGitLab(t, map[string]handler{
		"/wikis":    statusResponse(http.StatusForbidden),
		"/snippets": jsonResponse(`[]`),
	})
	defer srv.Close()

	repos := listRepos(t, srv.URL)

	require.NotContains(t, paths(repos), "team/project.wiki")
}

func TestForbiddenSnippetsTreatedAsNoSnippets(t *testing.T) {
	t.Parallel()
	srv := fakeGitLab(t, map[string]handler{
		"/wikis":    jsonResponse(`[]`),
		"/snippets": statusResponse(http.StatusForbidden),
	})
	defer srv.Close()

	_, err := clientFor(t, srv.URL).ListRepos(context.Background(), backup.StateAll)

	require.NoError(t, err)
}

func TestSnippetsPageThroughEveryResult(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := fakeGitLab(t, map[string]handler{
		"/wikis": jsonResponse(`[]`),
		"/snippets": func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("x-next-page", "2")
				_, _ = w.Write([]byte(`[{"id":1}]`))

				return
			}
			_, _ = w.Write([]byte(`[{"id":2}]`))
		},
	})
	defer srv.Close()

	repos := listRepos(t, srv.URL)

	require.Contains(t, paths(repos), "team/project/snippets/1")
	require.Contains(t, paths(repos), "team/project/snippets/2")
	require.Equal(t, 2, calls, "expected snippetRepos to follow x-next-page across two requests")
}

func TestUnexpectedStatusFromWikisFails(t *testing.T) {
	t.Parallel()
	srv := fakeGitLab(t, map[string]handler{
		"/wikis":    statusResponse(http.StatusInternalServerError),
		"/snippets": jsonResponse(`[]`),
	})
	defer srv.Close()

	_, err := clientFor(t, srv.URL).ListRepos(context.Background(), backup.StateAll)

	require.Error(t, err)
}

type handler = http.HandlerFunc

func jsonResponse(body string) handler {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func statusResponse(code int) handler {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
	}
}

// fakeGitLab serves one project, "team/project", from GET /api/v4/projects
// (archived=false only -- StateAll's archived=true pass gets an empty
// list), and routes a per-project sub-resource request to the handler
// named by its suffix ("/wikis" or "/snippets").
func fakeGitLab(t *testing.T, subHandlers map[string]handler) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for suffix, h := range subHandlers {
			if strings.HasSuffix(r.URL.Path, suffix) {
				h(w, r)

				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("archived") == "true" {
			_, _ = w.Write([]byte(`[]`))

			return
		}
		_, _ = w.Write([]byte(`[{"path_with_namespace":"team/project","archived":false,"empty_repo":false}]`))
	}))
}

func clientFor(t *testing.T, baseURL string) *gitlab.Client {
	t.Helper()
	client, err := gitlab.New(baseURL, "unused")
	require.NoError(t, err)

	return client
}

func listRepos(t *testing.T, baseURL string) []backup.Repo {
	t.Helper()
	repos, err := clientFor(t, baseURL).ListRepos(context.Background(), backup.StateAll)
	require.NoError(t, err)

	return repos
}

func paths(repos []backup.Repo) []string {
	out := make([]string, len(repos))
	for i, r := range repos {
		out[i] = r.Path
	}

	return out
}
