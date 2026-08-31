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

// notesPageSize mirrors gitlab.go's own unexported pageSize (100):
// duplicated here rather than exported, since a fixed page size is exactly
// what this test needs to control to prove pagination, not something the
// production adapter should expose just for a test to read.
const notesPageSize = 100

// issueUnitServer replies to the issues list with a single issue (iid 1,
// GitLab's own "opened" state) and to that issue's notes with
// notesPageSize notes on page 1 -- one of them a system-generated entry,
// which must be filtered out -- and one more, real, note on page 2. GitLab
// pages by the "x-next-page" response header rather than a length
// comparison the way Forgejo and GitHub do, so this is what actually
// proves the exporter follows a second page rather than stopping at the
// first: both the system-note filter and the pagination itself only show
// up correctly if the final comment count is notesPageSize exactly
// (notesPageSize-1 real notes from page 1, plus 1 from page 2).
func issueUnitServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, "/notes"):
			writeNotesPage(w, r)
		case strings.HasSuffix(r.URL.Path, "/issues"):
			writeIssuesPage(w, r)
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
}

// writeNotesPage replies with notesPageSize notes on page 1 -- one of them
// a system-generated entry, which must be filtered out -- and one more,
// real, note on page 2, proving both the system-note filter and that the
// exporter follows GitLab's "x-next-page" header rather than stopping at
// the first page.
func writeNotesPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("page") == "2" {
		_, _ = w.Write([]byte(
			`[{"author":{"username":"bob"},"body":"a real comment on page 2","created_at":"2026-08-15T09:00:00Z","system":false}]`))

		return
	}

	w.Header().Set("x-next-page", "2")
	var b strings.Builder
	b.WriteByte('[')
	for i := range notesPageSize {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"author":{"username":"alice"},"body":"note %d","created_at":"2026-08-15T08:00:00Z","system":%t}`,
			i+1, i == 0)
	}
	b.WriteByte(']')
	_, _ = w.Write([]byte(b.String()))
}

// writeIssuesPage replies to page 1 with a single issue (iid 1, GitLab's
// own "opened" state) and to every other page with an empty list.
func writeIssuesPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("page") != "" && r.URL.Query().Get("page") != "1" {
		_, _ = w.Write([]byte(`[]`))

		return
	}
	_, _ = w.Write([]byte(`[{"iid":1,"title":"an issue","description":"body text",` +
		`"author":{"username":"alice"},"state":"opened","labels":["bug"],` +
		`"created_at":"2026-08-15T08:00:00Z","updated_at":"2026-08-15T09:00:00Z","closed_at":null}]`))
}

func TestIssueExporterUnit(t *testing.T) {
	srv := issueUnitServer(t)
	defer srv.Close()

	client, err := gitlab.New(srv.URL, "unused")
	require.NoError(t, err)

	exp := gitlab.NewIssueExporter(client)
	require.Equal(t, backup.MetadataIssues, exp.Kind())

	dir := t.TempDir()
	require.NoError(t, exp.Export(t.Context(), backup.Repo{Path: "team/repo"}, dir))

	data, err := os.ReadFile(filepath.Join(dir, "1.json"))
	require.NoError(t, err)

	var issue backup.Issue
	require.NoError(t, json.Unmarshal(data, &issue))

	require.Equal(t, "open", issue.State, `GitLab's "opened" must be normalized to "open"`)
	require.Equal(t, []string{"bug"}, issue.Labels)
	require.Equal(t, "alice", issue.Author)
	require.Nil(t, issue.ClosedAt)

	require.Len(t, issue.Comments, notesPageSize,
		"expected every non-system note across both pages: %d real from page 1, 1 from page 2", notesPageSize-1)
	require.Equal(t, "a real comment on page 2", issue.Comments[notesPageSize-1].Body)
}
