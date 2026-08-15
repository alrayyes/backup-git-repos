//go:build integration && gitlab

package gitlab_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/exec"
)

// rootToken is a fixed value: gitlab-rails runner sets it directly, so
// seeding never has to parse a generated token out of CLI output the way
// Forgejo's harness does.
const rootToken = "glpat-testtesttesttesttest"

// fixture is a running GitLab instance seeded with a known set of projects
// under a "team" group: an active one carrying an extra branch and tag, an
// archived one, and an empty one.
type fixture struct {
	BaseURL string
	Token   string
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
// team/active-repo with an extra branch and tag, team/archived-repo
// (archived after creation, since the create endpoint has no such field),
// and team/empty-repo.
func (f fixture) seed(t *testing.T) {
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

	f.post(t, "/api/v4/projects",
		map[string]any{"name": "archived-repo", "namespace_id": group.ID, "initialize_with_readme": true}, nil)
	f.post(t, "/api/v4/projects/team%2Farchived-repo/archive", nil, nil)

	f.post(t, "/api/v4/projects",
		map[string]any{"name": "empty-repo", "namespace_id": group.ID}, nil)
}

// post sends an authenticated JSON POST and requires a 2xx response,
// decoding the body into out when it's non-nil.
func (f fixture) post(t *testing.T, path string, body, out any) {
	t.Helper()
	f.do(t, http.MethodPost, path, body, out)
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
