//go:build integration

package github_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/github"
	"github.com/stretchr/testify/require"
)

// issueServer replays testdata/issues.json and its two comment fixtures,
// the same recorded-fixture approach TestRecordedListerContract uses --
// GitHub.com has no self-hostable instance, so this is the only fixture
// this adapter will ever have. issues.json carries a third item, a pull
// request (its "pull_request" field set), proving the exporter's filter:
// GitHub's issues endpoint returns pull requests alongside real issues,
// unlike GitLab's, which keeps merge requests on an entirely separate
// endpoint.
func issueServer(t *testing.T) *httptest.Server {
	t.Helper()

	issues := readFixture(t, "issues.json")
	comments := map[string][]byte{
		"1": readFixture(t, "issue_1_comments.json"),
		"2": readFixture(t, "issue_2_comments.json"),
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "/comments") {
			number := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/team/active-repo/issues/"), "/comments")
			_, _ = w.Write(comments[number])

			return
		}

		if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte("[]"))

			return
		}
		_, _ = w.Write(issues)
	}))
}

func TestRecordedIssueExporterContract(t *testing.T) {
	t.Parallel()

	srv := issueServer(t)
	defer srv.Close()

	backup.TestIssueExporter(t, func(*testing.T) backup.MetadataExporter {
		client, err := github.New(srv.URL, "unused")
		require.NoError(t, err)

		return github.NewIssueExporter(client)
	})
}

// TestIssueExporterExcludesPullRequests guards #81's own hint directly:
// issues.json's third item is a pull request, and it must never be written
// out as an issue.
func TestIssueExporterExcludesPullRequests(t *testing.T) {
	t.Parallel()

	srv := issueServer(t)
	defer srv.Close()

	client, err := github.New(srv.URL, "unused")
	require.NoError(t, err)

	dir := t.TempDir()
	err = github.NewIssueExporter(client).Export(t.Context(), backup.Repo{Path: "team/active-repo"}, dir)
	require.NoError(t, err)

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	require.Len(t, entries, 2, "expected exactly the two real issues, no pull request among them")
}
