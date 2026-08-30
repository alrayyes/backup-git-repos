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

// commentsPageSize mirrors github.go's own unexported pageSize (100):
// duplicated here rather than exported, since a fixed page size is exactly
// what this test needs to control to prove pagination, not something the
// production adapter should expose just for a test to read.
const commentsPageSize = 100

// issuePaginationServer replies to the issues list with a single issue
// (number 1), and to that issue's comments with exactly commentsPageSize
// comments on page 1 and one more, distinguishably named, on page 2 --
// proving fetchComments actually follows a second page rather than
// silently stopping at GitHub's own default page size (30).
func issuePaginationServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(r.URL.Path, "/issues") {
			if r.URL.Query().Get("page") != "1" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{"number":1,"title":"an issue with many comments","user":{"login":"alice"},"state":"open"}]`))
			return
		}

		page := r.URL.Query().Get("page")
		if page == "2" {
			_, _ = w.Write([]byte(`[{"user":{"login":"bob"},"body":"comment 101 (from page 2)"}]`))
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
			fmt.Fprintf(&b, `{"user":{"login":"alice"},"body":"comment %d"}`, i+1)
		}
		b.WriteByte(']')
		_, _ = w.Write([]byte(b.String()))
	}))
}

func TestIssueExporterPaginatesComments(t *testing.T) {
	srv := issuePaginationServer(t)
	defer srv.Close()

	client, err := github.New(srv.URL, "unused")
	require.NoError(t, err)

	dir := t.TempDir()
	err = github.NewIssueExporter(client).Export(t.Context(), backup.Repo{Path: "team/repo"}, dir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "1.json"))
	require.NoError(t, err)

	var issue backup.Issue
	require.NoError(t, json.Unmarshal(data, &issue))

	require.Len(t, issue.Comments, commentsPageSize+1,
		"expected every comment across both pages, not just the first page's %d", commentsPageSize)
	require.Equal(t, "comment 101 (from page 2)", issue.Comments[commentsPageSize].Body)
}
