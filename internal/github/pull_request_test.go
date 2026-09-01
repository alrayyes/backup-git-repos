//go:build integration

package github_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/github"
	"github.com/stretchr/testify/require"
)

// pullRequestServer replays testdata/pulls.json and its two comment
// fixtures, the same recorded-fixture approach issueServer uses --
// hand-authored against GitHub's REST API reference rather than captured
// from a live instance, since GitHub.com has no self-hostable equivalent
// (see CONTRIBUTING.md).
func pullRequestServer(t *testing.T) *httptest.Server {
	t.Helper()

	pulls := readFixture(t, "pulls.json")
	comments := map[string][]byte{
		"1": readFixture(t, "pull_1_comments.json"),
		"2": readFixture(t, "pull_2_comments.json"),
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "/comments") {
			number := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/team/active-repo/pulls/"), "/comments")
			_, _ = w.Write(comments[number])

			return
		}

		if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte("[]"))

			return
		}
		_, _ = w.Write(pulls)
	}))
}

func TestRecordedPullRequestExporterContract(t *testing.T) {
	srv := pullRequestServer(t)
	defer srv.Close()

	backup.TestPullRequestExporter(t, func(*testing.T) backup.MetadataExporter {
		client, err := github.New(srv.URL, "unused")
		require.NoError(t, err)

		return github.NewPullRequestExporter(client)
	})
}
