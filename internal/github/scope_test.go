package github_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/github"
	"github.com/stretchr/testify/require"
)

// scopeServer replies to GET /user/repos with a single public repo, the
// same response GitHub itself sends a token missing the repo scope, and
// carries the given X-OAuth-Scopes header -- the only signal GitHub gives
// for a classic personal access token's granted scopes.
func scopeServer(t *testing.T, scopes string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if scopes != "" {
			w.Header().Set("X-OAuth-Scopes", scopes)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte("[]"))

			return
		}
		_, _ = w.Write([]byte(`[{"full_name":"team/public-repo","archived":false,"size":1}]`))
	}))
}

func TestListReposRejectsTokenMissingRepoScope(t *testing.T) {
	srv := scopeServer(t, "public_repo, gist")
	defer srv.Close()

	client, err := github.New(srv.URL, "unused")
	require.NoError(t, err)

	_, err = client.ListRepos(t.Context(), backup.StateAll)
	require.ErrorIs(t, err, github.ErrMissingRepoScope)
}

func TestListReposAllowsTokenWithRepoScope(t *testing.T) {
	srv := scopeServer(t, "repo, gist")
	defer srv.Close()

	client, err := github.New(srv.URL, "unused")
	require.NoError(t, err)

	repos, err := client.ListRepos(t.Context(), backup.StateAll)
	require.NoError(t, err)
	require.Len(t, repos, 1)
}

func TestListReposSkipsScopeCheckWhenHeaderAbsent(t *testing.T) {
	// A fine-grained token gets no X-OAuth-Scopes header at all -- there's
	// nothing to check it against, so the request must still succeed.
	srv := scopeServer(t, "")
	defer srv.Close()

	client, err := github.New(srv.URL, "unused")
	require.NoError(t, err)

	repos, err := client.ListRepos(t.Context(), backup.StateAll)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	require.NotErrorIs(t, err, github.ErrMissingRepoScope)
}
