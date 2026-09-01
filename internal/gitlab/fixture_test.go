//go:build integration && gitlab

package gitlab_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/exec"
)

// rootToken is a fixed value: gitlab-rails runner sets it directly, so
// seeding never has to parse a generated token out of CLI output the way
// Forgejo's harness does.
const rootToken = "glpat-testtesttesttesttest"

// fixture is a running GitLab instance seeded with a known set of projects
// under a "team" group: an active one carrying an extra branch and tag, a
// wiki page and a snippet, an archived one, and an empty one -- the
// archived and empty projects carry no wiki page or snippet, which is what
// proves neither grows an empty entry.
type fixture struct {
	BaseURL string
	Token   string

	// SnippetPath is the path of the snippet seeded under the active
	// project, computed from the ID GitLab assigned it rather than
	// hardcoded, since that ID isn't otherwise predictable.
	SnippetPath string
}

// start boots a GitLab container, mints an API token for it, and seeds the
// fixtures every test in this package expects.
func start(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()

	ctr := runGitLab(ctx, t)

	baseURL, err := ctr.PortEndpoint(ctx, "80/tcp", "http")
	require.NoError(t, err)

	mintToken(ctx, t, ctr)

	f := fixture{BaseURL: baseURL, Token: rootToken}
	f.seed(t)

	return f
}

// mintToken runs a Rails console script and sets a fixed token value
// directly. GitLab's CLI has no equivalent of Forgejo's
// generate-access-token, so there's nothing to parse out of the output.
func mintToken(ctx context.Context, t *testing.T, ctr testcontainers.Container) {
	t.Helper()

	script := fmt.Sprintf(`
u = User.find_by_username('root')
t = u.personal_access_tokens.create!(scopes: ['api', 'read_repository'], name: 'backup-it', expires_at: 30.days.from_now)
t.set_token(%q)
t.save!
`, rootToken)

	code, output, err := ctr.Exec(ctx, []string{"gitlab-rails", "runner", script}, exec.Multiplexed())
	require.NoError(t, err)

	data, err := io.ReadAll(output)
	require.NoError(t, err)
	require.Zero(t, code, "gitlab-rails runner: %s", data)
}

// seed creates the group and projects every test in this package expects:
// team/active-repo with an extra branch and tag, a wiki page and a
// snippet; team/archived-repo (archived after creation, since the create
// endpoint has no such field); and team/empty-repo. Neither archived-repo
// nor empty-repo gets a wiki page or a snippet, which is what proves a
// project with neither doesn't grow an empty entry for either.
func (f *fixture) seed(t *testing.T) {
	t.Helper()

	var group struct {
		ID int `json:"id"`
	}
	f.post(t, "/api/v4/groups", map[string]any{"name": "team", "path": "team"}, &group)

	f.post(t, "/api/v4/projects",
		map[string]any{"name": "active-repo", "namespace_id": group.ID, "initialize_with_readme": true}, nil)
	f.post(t, "/api/v4/projects/team%2Factive-repo/repository/branches",
		map[string]any{"branch": "feature", "ref": "main"}, nil)
	f.post(t, "/api/v4/projects/team%2Factive-repo/repository/tags",
		map[string]any{"tag_name": "v1.0.0", "ref": "main"}, nil)
	f.post(t, "/api/v4/projects/team%2Factive-repo/wikis",
		map[string]any{"title": "Home", "content": "hello wiki"}, nil)
	f.seedIssues(t)
	f.seedReleases(t)

	var snippet struct {
		ID int `json:"id"`
	}
	f.post(t, "/api/v4/projects/team%2Factive-repo/snippets",
		map[string]any{"title": "snip", "file_name": "snip.txt", "content": "hello snippet", "visibility": "private"},
		&snippet)
	f.SnippetPath = fmt.Sprintf("%s/snippets/%d", backup.TestActiveRepoPath, snippet.ID)

	f.post(t, "/api/v4/projects",
		map[string]any{"name": "archived-repo", "namespace_id": group.ID, "initialize_with_readme": true}, nil)
	f.post(t, "/api/v4/projects/team%2Farchived-repo/archive", nil, nil)

	f.post(t, "/api/v4/projects",
		map[string]any{"name": "empty-repo", "namespace_id": group.ID}, nil)
}

// seedIssues creates the two issues backup.TestIssueExporter expects on
// team/active-repo: an open one carrying one comment
// (backup.TestIssueCommentBody), and a closed one carrying none -- proving
// both a populated comment list and #81's "an issue with no comments is
// still written" requirement from the one fixture set. Notes' own "system"
// note GitLab generates for the state-close ("changed status to closed")
// is left in place deliberately: IssueExporter is expected to filter it out
// on its own, and this fixture is what proves that rather than assuming it.
func (f fixture) seedIssues(t *testing.T) {
	t.Helper()

	var open struct {
		IID int `json:"iid"`
	}
	f.post(t, "/api/v4/projects/team%2Factive-repo/issues",
		map[string]any{"title": backup.TestIssueOpenTitle, "description": "please fix this"}, &open)
	f.post(t, fmt.Sprintf("/api/v4/projects/team%%2Factive-repo/issues/%d/notes", open.IID),
		map[string]any{"body": backup.TestIssueCommentBody}, nil)

	var closed struct {
		IID int `json:"iid"`
	}
	f.post(t, "/api/v4/projects/team%2Factive-repo/issues",
		map[string]any{"title": backup.TestIssueClosedTitle, "description": "already handled"}, &closed)
	f.put(t, fmt.Sprintf("/api/v4/projects/team%%2Factive-repo/issues/%d", closed.IID),
		map[string]any{"state_event": "close"})
}

// seedReleases creates the two releases backup.TestReleaseExporter expects
// on team/active-repo: one tagged backup.TestReleaseTagWithAsset carrying
// one uploaded asset (backup.TestReleaseAssetName /
// backup.TestReleaseAssetContent), and one tagged
// backup.TestReleaseTagNoAssets carrying none -- proving both a downloaded
// asset's content and the "no empty asset files" requirement from the one
// fixture set. An "uploaded asset" on GitLab means uploading the file to
// the project first (POST .../uploads, the same endpoint markdown image
// attachments use), then creating the release with an assets.links entry
// pointing at that upload's own URL -- unlike Forgejo's release-asset
// endpoint, GitLab's release API takes only links, never a file directly.
func (f fixture) seedReleases(t *testing.T) {
	t.Helper()

	assetURL := f.postUpload(t, "/api/v4/projects/team%2Factive-repo/uploads",
		backup.TestReleaseAssetName, backup.TestReleaseAssetContent)

	f.post(t, "/api/v4/projects/team%2Factive-repo/releases", map[string]any{
		"tag_name": backup.TestReleaseTagWithAsset, "ref": "main",
		"name": "v1.0.0", "description": "release notes",
		"assets": map[string]any{
			"links": []map[string]any{{"name": backup.TestReleaseAssetName, "url": assetURL}},
		},
	}, nil)

	f.post(t, "/api/v4/projects/team%2Factive-repo/releases", map[string]any{
		"tag_name": backup.TestReleaseTagNoAssets, "ref": "main",
		"name": "v0.9.0", "description": "no assets here",
	}, nil)
}

// post sends an authenticated JSON POST and requires a 2xx response,
// decoding the body into out when it's non-nil.
func (f fixture) post(t *testing.T, path string, body, out any) {
	t.Helper()
	f.do(t, http.MethodPost, path, body, out)
}

// postUpload uploads content as name via GitLab's project uploads endpoint
// (a multipart/form-data POST, the "file" field it expects) and returns the
// absolute URL the release's own assets.links entry should point at --
// full_path is project-relative ("/team/active-repo/uploads/<hash>/name"),
// so it's joined against f.BaseURL here rather than left for the caller to
// resolve.
func (f fixture) postUpload(t *testing.T, path, name, content string) string {
	t.Helper()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", name)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, f.BaseURL+path, &body)
	require.NoError(t, err)
	req.Header.Set("PRIVATE-TOKEN", f.Token)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Less(t, resp.StatusCode, 300, "POST %s", path)

	var uploaded struct {
		FullPath string `json:"full_path"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&uploaded))

	return f.BaseURL + uploaded.FullPath
}

// put sends an authenticated JSON PUT and requires a 2xx response.
func (f fixture) put(t *testing.T, path string, body any) {
	t.Helper()
	f.do(t, http.MethodPut, path, body, nil)
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
	req.Header.Set("PRIVATE-TOKEN", f.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Less(t, resp.StatusCode, 300, "%s %s", method, path)

	if out != nil {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
	}
}
