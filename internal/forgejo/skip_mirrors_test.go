package forgejo_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/stretchr/testify/require"
)

// searchServer replies to GET /api/v1/repos/search with a fixed page: one
// ordinary repo and one Forgejo reports as a mirror.
func searchServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[
			{"full_name":"team/active-repo","archived":false,"empty":false,"mirror":false},
			{"full_name":"team/mirror-repo","archived":false,"empty":false,"mirror":true}
		]}`))
	}))
}

func TestListReposLogsWhichRepoSkipMirrorsExcluded(t *testing.T) {
	srv := searchServer(t)
	defer srv.Close()

	client, err := forgejo.New(srv.URL, "unused")
	require.NoError(t, err)
	client.SkipMirrors = true

	var logs bytes.Buffer
	client.SetLogger(slog.New(slog.NewTextHandler(&logs, nil)))

	repos, err := client.ListRepos(context.Background(), backup.StateAll)
	require.NoError(t, err)

	require.Len(t, repos, 1)
	require.Equal(t, "team/active-repo", repos[0].Path)
	require.Contains(t, logs.String(), "team/mirror-repo")
}

func TestListReposLogsNothingWhenSkipMirrorsIsOff(t *testing.T) {
	srv := searchServer(t)
	defer srv.Close()

	client, err := forgejo.New(srv.URL, "unused")
	require.NoError(t, err)

	var logs bytes.Buffer
	client.SetLogger(slog.New(slog.NewTextHandler(&logs, nil)))

	repos, err := client.ListRepos(context.Background(), backup.StateAll)
	require.NoError(t, err)

	require.Len(t, repos, 2)
	require.Empty(t, logs.String())
}
