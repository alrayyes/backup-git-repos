package gitlab_test

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
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
	"github.com/stretchr/testify/require"
)

// mergeRequestPageSize mirrors gitlab.go's own unexported pageSize (100),
// the same reasoning notesPageSize (issue_unit_test.go) already gives for
// duplicating it here rather than exporting it.
const mergeRequestPageSize = 100

// mergeRequestUnitServer replies to the merge requests list with a single
// merge request (iid 1, GitLab's own "opened" state) and to that merge
// request's discussions with mergeRequestPageSize discussions on page 1 --
// one of them system-generated, which must be filtered out, and one a
// plain (non-diff) comment -- and one more, diff-anchored, discussion on
// page 2. Proves the system-note filter, the diff-anchor mapping, and that
// the exporter follows GitLab's "x-next-page" header rather than stopping
// at the first page, the same three things issueUnitServer already proves
// for issue notes.
func mergeRequestUnitServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, "/discussions"):
			writeDiscussionsPage(w, r)
		case strings.HasSuffix(r.URL.Path, "/merge_requests"):
			writeMergeRequestsPage(w, r)
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
}

func writeDiscussionsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("page") == "2" {
		_, _ = w.Write([]byte(`[{"notes":[{"type":"DiffNote","author":{"username":"carol"},` +
			`"body":"an inline comment","created_at":"2026-08-15T09:00:00Z","system":false,` +
			`"position":{"new_path":"main.go","new_line":7,"old_path":"main.go","old_line":null}}]}]`))

		return
	}

	w.Header().Set("x-next-page", "2")
	var b strings.Builder
	b.WriteByte('[')
	for i := range mergeRequestPageSize {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"notes":[{"type":"DiscussionNote","author":{"username":"alice"},`+
			`"body":"note %d","created_at":"2026-08-15T08:00:00Z","system":%t,"position":null}]}`,
			i+1, i == 0)
	}
	b.WriteByte(']')
	_, _ = w.Write([]byte(b.String()))
}

func writeMergeRequestsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
		_, _ = w.Write([]byte(`[]`))

		return
	}
	_, _ = w.Write([]byte(`[{"iid":1,"title":"a merge request","description":"body text",` +
		`"author":{"username":"alice"},"state":"opened","source_branch":"feature","target_branch":"main",` +
		`"created_at":"2026-08-15T08:00:00Z","updated_at":"2026-08-15T09:00:00Z","closed_at":null,"merged_at":null}]`))
}

func TestPullRequestExporterUnit(t *testing.T) {
	srv := mergeRequestUnitServer(t)
	defer srv.Close()

	client, err := gitlab.New(srv.URL, "unused")
	require.NoError(t, err)

	exp := gitlab.NewPullRequestExporter(client)
	require.Equal(t, backup.MetadataPullRequests, exp.Kind())

	dir := t.TempDir()
	require.NoError(t, exp.Export(t.Context(), backup.Repo{Path: "team/repo"}, dir))

	data, err := os.ReadFile(filepath.Join(dir, "1.json"))
	require.NoError(t, err)

	var pr backup.PullRequest
	require.NoError(t, json.Unmarshal(data, &pr))

	require.Equal(t, "open", pr.State, `GitLab's "opened" must be normalized to "open"`)
	require.Equal(t, "alice", pr.Author)
	require.Equal(t, "feature", pr.SourceBranch)
	require.Equal(t, "main", pr.TargetBranch)
	require.Nil(t, pr.ClosedAt)
	require.Nil(t, pr.MergedAt)

	require.Len(t, pr.Comments, mergeRequestPageSize,
		"expected every non-system note across both pages: %d real from page 1, 1 from page 2", mergeRequestPageSize-1)

	last := pr.Comments[mergeRequestPageSize-1]
	require.Equal(t, "an inline comment", last.Body)
	require.Equal(t, "main.go", last.File)
	require.Equal(t, 7, last.Line)

	require.Empty(t, pr.Comments[0].File, "a plain discussion comment must not carry a diff anchor")
}
