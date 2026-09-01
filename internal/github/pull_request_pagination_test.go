package github_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/github"
	"github.com/stretchr/testify/require"
)

// pullRequestPaginationServer replies to the pulls list with a single pull
// request (number 1), and to that pull request's review comments with
// exactly commentsPageSize comments on page 1 and one more, distinguishably
// named, on page 2 -- proving fetchReviewComments actually follows a second
// page rather than silently stopping at GitHub's own default page size (30).
func pullRequestPaginationServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(r.URL.Path, "/pulls") {
			if r.URL.Query().Get("page") != "1" {
				_, _ = w.Write([]byte(`[]`))

				return
			}
			_, _ = w.Write([]byte(`[{"number":1,"title":"a pull request with many comments","user":{"login":"alice"},` +
				`"state":"open","head":{"ref":"feature"},"base":{"ref":"main"}}]`))

			return
		}

		page := r.URL.Query().Get("page")
		if page == "2" {
			_, _ = w.Write([]byte(
				`[{"user":{"login":"bob"},"body":"comment 101 (from page 2)","path":"main.go","line":42}]`))

			return
		}
		if page != "1" {
			_, _ = w.Write([]byte(`[]`))

			return
		}

		var b strings.Builder
		b.WriteByte('[')
		for i := range commentsPageSize {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"user":{"login":"alice"},"body":"comment %d","path":"main.go","line":%d}`, i+1, i+1)
		}
		b.WriteByte(']')
		_, _ = w.Write([]byte(b.String()))
	}))
}

func TestPullRequestExporterPaginatesComments(t *testing.T) {
	srv := pullRequestPaginationServer(t)
	defer srv.Close()

	client, err := github.New(srv.URL, "unused")
	require.NoError(t, err)

	exp := github.NewPullRequestExporter(client)
	require.Equal(t, backup.MetadataPullRequests, exp.Kind())

	dir := t.TempDir()
	err = exp.Export(t.Context(), backup.Repo{Path: "team/repo"}, dir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "1.json"))
	require.NoError(t, err)

	var pr backup.PullRequest
	require.NoError(t, json.Unmarshal(data, &pr))

	require.Len(t, pr.Comments, commentsPageSize+1,
		"expected every comment across both pages, not just the first page's %d", commentsPageSize)
	require.Equal(t, "comment 101 (from page 2)", pr.Comments[commentsPageSize].Body)
	require.Equal(t, "main.go", pr.Comments[commentsPageSize].File)
	require.Equal(t, 42, pr.Comments[commentsPageSize].Line)
}
