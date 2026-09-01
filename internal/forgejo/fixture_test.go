//go:build integration

package forgejo_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/exec"
	tcforgejo "github.com/testcontainers/testcontainers-go/modules/forgejo"
)

// fixture is a running Forgejo instance seeded with a known set of
// repositories: an active one carrying an extra branch and tag, an archived
// one, an empty one, and a personal one under the admin account.
type fixture struct {
	BaseURL       string
	Token         string
	AdminUsername string
}

// start boots a Forgejo container, mints an API token for it, and seeds the
// fixtures every test in this package expects.
func start(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()

	// Forgejo refuses to migrate/mirror from a host it treats as local
	// network by default (SSRF protection); migrateMirror needs that
	// relaxed to point a mirror at another repo on the instance itself.
	ctr, err := tcforgejo.Run(ctx, image, tcforgejo.WithConfig("migrations", "ALLOW_LOCALNETWORKS", "true"))
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	baseURL, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)

	f := fixture{
		BaseURL:       baseURL,
		Token:         mintToken(ctx, t, ctr),
		AdminUsername: ctr.AdminUsername(),
	}
	f.seed(t)

	return f
}

// mintToken runs the CLI command as the "git" user, which is required since
// Forgejo refuses to run it as root, and returns the raw token.
func mintToken(ctx context.Context, t *testing.T, ctr *tcforgejo.Container) string {
	t.Helper()

	code, output, err := ctr.Exec(ctx, []string{
		"forgejo", "admin", "user", "generate-access-token",
		"-u", ctr.AdminUsername(),
		"-t", "backup-it",
		"--raw",
		"--scopes", "all",
	}, exec.WithUser("git"), exec.Multiplexed())
	require.NoError(t, err)

	data, err := io.ReadAll(output)
	require.NoError(t, err)
	require.Zero(t, code, "generate-access-token: %s", data)

	return strings.TrimSpace(string(data))
}

// seed creates the org and repositories every test in this package expects:
// team/active-repo with an extra branch and tag, team/archived-repo (marked
// archived after creation, since the create endpoint has no such field),
// team/empty-repo, and a personal repository under the admin account.
// team/active-repo also gets the two issues backup.TestIssueExporter
// expects (see seedIssues below), the two pull requests
// backup.TestPullRequestExporter expects (see seedPullRequests below) and,
// to prove #81's issues-only filter, a bare pull request with neither --
// Forgejo's issues endpoint returns pull requests alongside real issues
// unless the exporter asks for type=issues, so a pull request among the
// fixtures is what would catch a regression there.
func (f fixture) seed(t *testing.T) {
	t.Helper()

	f.post(t, "/api/v1/orgs", map[string]any{"username": "team"}, nil)

	f.post(t, "/api/v1/orgs/team/repos", map[string]any{"name": "active-repo", "auto_init": true}, nil)
	f.post(t, "/api/v1/repos/team/active-repo/branches",
		map[string]any{"new_branch_name": "feature", "old_ref_name": "main"}, nil)
	f.post(t, "/api/v1/repos/team/active-repo/tags",
		map[string]any{"tag_name": "v1.0.0", "target": "main"}, nil)
	f.seedIssues(t)
	f.seedPullRequest(t)
	f.seedReleases(t)
	f.seedPullRequests(t)

	f.post(t, "/api/v1/orgs/team/repos", map[string]any{"name": "archived-repo", "auto_init": true}, nil)
	f.patch(t, "/api/v1/repos/team/archived-repo", map[string]any{"archived": true})

	f.post(t, "/api/v1/orgs/team/repos", map[string]any{"name": "empty-repo"}, nil)

	f.post(t, "/api/v1/user/repos", map[string]any{"name": "personal", "auto_init": true}, nil)
}

// seedIssues creates the two issues backup.TestIssueExporter expects on
// team/active-repo: an open one carrying one comment
// (backup.TestIssueCommentBody), and a closed one carrying none -- proving
// both a populated comment list and #81's "an issue with no comments is
// still written" requirement from the one fixture set.
func (f fixture) seedIssues(t *testing.T) {
	t.Helper()

	f.post(t, "/api/v1/repos/team/active-repo/issues",
		map[string]any{"title": backup.TestIssueOpenTitle, "body": "please fix this"}, nil)
	f.post(t, "/api/v1/repos/team/active-repo/issues/1/comments",
		map[string]any{"body": backup.TestIssueCommentBody}, nil)

	f.post(t, "/api/v1/repos/team/active-repo/issues",
		map[string]any{"title": backup.TestIssueClosedTitle, "body": "already handled"}, nil)
	f.patch(t, "/api/v1/repos/team/active-repo/issues/2", map[string]any{"state": "closed"})
}

// seedPullRequest creates a branch and opens a pull request from it against
// team/active-repo's default branch -- see seed's own doc comment for why.
func (f fixture) seedPullRequest(t *testing.T) {
	t.Helper()

	f.post(t, "/api/v1/repos/team/active-repo/branches",
		map[string]any{"new_branch_name": "pr-branch", "old_ref_name": "main"}, nil)
	f.post(t, "/api/v1/repos/team/active-repo/pulls",
		map[string]any{"title": "a pull request", "head": "pr-branch", "base": "main"}, nil)
}

// seedReleases creates the two releases backup.TestReleaseExporter expects
// on team/active-repo: one tagged backup.TestReleaseTagWithAsset carrying
// one uploaded asset (backup.TestReleaseAssetName /
// backup.TestReleaseAssetContent), and one tagged
// backup.TestReleaseTagNoAssets carrying none -- proving both a downloaded
// asset's content and the "no empty asset files" requirement from the one
// fixture set.
func (f fixture) seedReleases(t *testing.T) {
	t.Helper()

	var withAsset struct {
		ID int `json:"id"`
	}
	f.post(t, "/api/v1/repos/team/active-repo/releases", map[string]any{
		"tag_name": backup.TestReleaseTagWithAsset, "target_commitish": "main",
		"name": "v1.0.0", "body": "release notes",
	}, &withAsset)
	f.postAsset(t, fmt.Sprintf("/api/v1/repos/team/active-repo/releases/%d/assets", withAsset.ID),
		backup.TestReleaseAssetName, backup.TestReleaseAssetContent)

	f.post(t, "/api/v1/repos/team/active-repo/releases", map[string]any{
		"tag_name": backup.TestReleaseTagNoAssets, "target_commitish": "main",
		"name": "v0.9.0", "body": "no assets here",
	}, nil)
}

// seedPullRequests creates the two pull requests backup.TestPullRequestExporter
// expects on team/active-repo: an open one that edits README.md and carries
// a single diff-anchored review comment on that file, at
// backup.TestPullRequestReviewCommentLine, and a merged one carrying none --
// proving both the diff-anchor mapping and #81's own "a pull/merge request
// with no comments is still written" requirement from the one fixture set.
func (f fixture) seedPullRequests(t *testing.T) {
	t.Helper()

	f.seedOpenPullRequestWithReviewComment(t)
	f.seedMergedPullRequest(t)
}

func (f fixture) seedOpenPullRequestWithReviewComment(t *testing.T) {
	t.Helper()

	f.post(t, "/api/v1/repos/team/active-repo/branches",
		map[string]any{"new_branch_name": "pr-open-branch", "old_ref_name": "main"}, nil)

	var content struct {
		SHA string `json:"sha"`
	}
	f.get(t, "/api/v1/repos/team/active-repo/contents/README.md?ref=pr-open-branch", &content)
	f.put(t, "/api/v1/repos/team/active-repo/contents/README.md", map[string]any{
		"branch":  "pr-open-branch",
		"sha":     content.SHA,
		"content": base64.StdEncoding.EncodeToString([]byte("active-repo\n")),
		"message": "edit readme",
	})

	var pr struct {
		Number int `json:"number"`
	}
	f.post(t, "/api/v1/repos/team/active-repo/pulls", map[string]any{
		"title": backup.TestPullRequestOpenTitle, "body": "please review this",
		"head": "pr-open-branch", "base": "main",
	}, &pr)

	f.post(t, fmt.Sprintf("/api/v1/repos/team/active-repo/pulls/%d/reviews", pr.Number), map[string]any{
		"event": "COMMENT",
		"comments": []map[string]any{{
			"path":         backup.TestPullRequestReviewCommentFile,
			"new_position": backup.TestPullRequestReviewCommentLine,
			"body":         backup.TestPullRequestReviewCommentBody,
		}},
	}, nil)
}

func (f fixture) seedMergedPullRequest(t *testing.T) {
	t.Helper()

	f.post(t, "/api/v1/repos/team/active-repo/branches",
		map[string]any{"new_branch_name": "pr-merge-branch", "old_ref_name": "main"}, nil)
	f.post(t, "/api/v1/repos/team/active-repo/contents/merged-file.txt", map[string]any{
		"branch":  "pr-merge-branch",
		"content": base64.StdEncoding.EncodeToString([]byte("content\n")),
		"message": "add a file",
	}, nil)

	var pr struct {
		Number int `json:"number"`
	}
	f.post(t, "/api/v1/repos/team/active-repo/pulls", map[string]any{
		"title": backup.TestPullRequestMergedTitle, "body": "already landed",
		"head": "pr-merge-branch", "base": "main",
	}, &pr)

	f.post(t, fmt.Sprintf("/api/v1/repos/team/active-repo/pulls/%d/merge", pr.Number),
		map[string]any{"Do": "merge"}, nil)
}

// post sends an authenticated JSON POST and requires a 2xx response,
// decoding the body into out when it's non-nil.
func (f fixture) post(t *testing.T, path string, body, out any) {
	t.Helper()
	f.do(t, http.MethodPost, path, body, out)
}

// postAsset uploads content as a release asset named name via a
// multipart/form-data POST -- the "attachment" field Forgejo's own release
// asset upload endpoint expects, since that endpoint takes a file upload
// rather than a JSON body the way every other seed helper here does.
func (f fixture) postAsset(t *testing.T, path, name, content string) {
	t.Helper()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("attachment", name)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		f.BaseURL+path+"?name="+url.QueryEscape(name), &body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "token "+f.Token)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Less(t, resp.StatusCode, 300, "POST %s", path)
}

// patch sends an authenticated JSON PATCH and requires a 2xx response.
func (f fixture) patch(t *testing.T, path string, body any) {
	t.Helper()
	f.do(t, http.MethodPatch, path, body, nil)
}

// put sends an authenticated JSON PUT and requires a 2xx response.
func (f fixture) put(t *testing.T, path string, body any) {
	t.Helper()
	f.do(t, http.MethodPut, path, body, nil)
}

// get sends an authenticated GET and decodes the response body into out.
func (f fixture) get(t *testing.T, path string, out any) {
	t.Helper()
	f.do(t, http.MethodGet, path, nil, out)
}

func (f fixture) do(t *testing.T, method, path string, body, out any) {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, f.BaseURL+path, reqBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "token "+f.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Less(t, resp.StatusCode, 300, "%s %s", method, path)

	if out != nil {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
	}
}
