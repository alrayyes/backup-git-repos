package forgejo_test

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
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/stretchr/testify/require"
)

// pullRequestPaginationServer replies to the pulls list with a single pull
// request (number 1) with one review (id 1), and to that review's own
// comments with exactly commentsPageSize diff-anchored comments -- proving
// fetchOneReviewComments carries every comment through rather than silently
// truncating at some smaller size. The reviews list itself replies with
// exactly commentsPageSize reviews on page 1 and one more, distinguishably
// bodied, on page 2, proving fetchReviewComments follows a second page of
// reviews the same way issue comments already do.
func pullRequestPaginationServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/reviews/") && strings.HasSuffix(r.URL.Path, "/comments"):
			writeReviewComments(w, r)
		case strings.HasSuffix(r.URL.Path, "/reviews"):
			writeReviewsPage(w, r)
		case strings.HasSuffix(r.URL.Path, "/pulls"):
			writePullsPage(w, r)
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
}

func writeReviewComments(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/reviews/1/comments") {
		_, _ = w.Write([]byte(`[]`))

		return
	}

	var b strings.Builder
	b.WriteByte('[')
	for i := range commentsPageSize {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"user":{"login":"alice"},"body":"inline comment %d","path":"main.go","position":%d}`, i+1, i+1)
	}
	b.WriteByte(']')
	_, _ = w.Write([]byte(b.String()))
}

func writeReviewsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("page") == "2" {
		_, _ = w.Write([]byte(
			`[{"id":2,"user":{"login":"carol"},"body":"a review left on page 2","submitted_at":"2026-08-15T09:00:00Z"}]`))

		return
	}

	var b strings.Builder
	b.WriteByte('[')
	for i := range commentsPageSize {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":%d,"user":{"login":"bob"},"body":"","submitted_at":"2026-08-15T08:00:00Z"}`, i+1)
	}
	b.WriteByte(']')
	_, _ = w.Write([]byte(b.String()))
}

func writePullsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("page") != "1" {
		_, _ = w.Write([]byte(`[]`))

		return
	}
	_, _ = w.Write([]byte(`[{"number":1,"title":"a pull request with many reviews","user":{"login":"alice"},` +
		`"state":"open","head":{"ref":"feature"},"base":{"ref":"main"}}]`))
}

func TestPullRequestExporterPaginatesReviews(t *testing.T) {
	srv := pullRequestPaginationServer(t)
	defer srv.Close()

	client, err := forgejo.New(srv.URL, "unused")
	require.NoError(t, err)

	exp := forgejo.NewPullRequestExporter(client)
	require.Equal(t, backup.MetadataPullRequests, exp.Kind())

	dir := t.TempDir()
	err = exp.Export(t.Context(), backup.Repo{Path: "team/repo"}, dir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "1.json"))
	require.NoError(t, err)

	var pr backup.PullRequest
	require.NoError(t, json.Unmarshal(data, &pr))

	// commentsPageSize reviews from page 1 (each carrying one inline
	// comment, only review 1's own comments endpoint is seeded so the rest
	// contribute nothing extra) plus one review body from page 2.
	require.Contains(t, pr.Comments, backup.ReviewComment{
		Author: "carol", Body: "a review left on page 2", CreatedAt: pr.Comments[len(pr.Comments)-1].CreatedAt,
	})

	var inline int
	for _, c := range pr.Comments {
		if c.File != "" {
			inline++
		}
	}
	require.Equal(t, commentsPageSize, inline, "expected every inline comment on review 1, not just the first page's worth")
}
