//go:build integration

package github_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/github"
	"github.com/stretchr/testify/require"
)

const testdataDir = "testdata"

// TestRecordedListerContract runs the same contract suite the Forgejo and
// GitLab adapters run against a real container, but against JSON recorded
// from GitHub's own REST API documentation instead: GitHub.com is a single
// SaaS instance, not something that can be booted in a container the way a
// self-hosted forge can, so a recording is the only fixture this adapter
// will ever have. Keep testdata/repos.json's shape true to what
// GET /user/repos actually returns if that endpoint's fields ever change.
func TestRecordedListerContract(t *testing.T) {
	t.Parallel()

	repos := readFixture(t, "repos.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte("[]"))

			return
		}
		_, _ = w.Write(repos)
	}))
	defer srv.Close()

	backup.TestLister(t, func(*testing.T) backup.Lister {
		client, err := github.New(srv.URL, "unused")
		require.NoError(t, err)

		return client
	})
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir, name))
	require.NoError(t, err)

	return data
}
