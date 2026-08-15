//go:build integration

package backup_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/forgejo"
)

// forgejoFixture is a running Forgejo instance seeded with a known set of
// repositories: an active one carrying an extra branch and tag, an archived
// one, an empty one, and a personal one under the admin account.
type forgejoFixture struct {
	BaseURL       string
	Token         string
	AdminUsername string
}

// startForgejo boots a Forgejo container, mints an API token for it, and
// seeds the fixtures every test in this package expects.
func startForgejo(t *testing.T) forgejoFixture {
	t.Helper()
	ctx := context.Background()

	ctr, err := forgejo.Run(ctx, forgejoImage)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	baseURL, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)

	f := forgejoFixture{
		BaseURL:       baseURL,
		Token:         mintForgejoToken(ctx, t, ctr),
		AdminUsername: ctr.AdminUsername(),
	}
	f.seed(t)

	return f
}

// mintForgejoToken runs the CLI command as the "git" user, which is required
// since Forgejo refuses to run it as root, and returns the raw token.
func mintForgejoToken(ctx context.Context, t *testing.T, ctr *forgejo.Container) string {
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
func (f forgejoFixture) seed(t *testing.T) {
	t.Helper()

	f.post(t, "/api/v1/orgs", map[string]any{"username": "team"}, nil)

	f.post(t, "/api/v1/orgs/team/repos", map[string]any{"name": "active-repo", "auto_init": true}, nil)
	f.post(t, "/api/v1/repos/team/active-repo/branches",
		map[string]any{"new_branch_name": "feature", "old_ref_name": "main"}, nil)
	f.post(t, "/api/v1/repos/team/active-repo/tags",
		map[string]any{"tag_name": "v1.0.0", "target": "main"}, nil)

	f.post(t, "/api/v1/orgs/team/repos", map[string]any{"name": "archived-repo", "auto_init": true}, nil)
	f.patch(t, "/api/v1/repos/team/archived-repo", map[string]any{"archived": true})

	f.post(t, "/api/v1/orgs/team/repos", map[string]any{"name": "empty-repo"}, nil)

	f.post(t, "/api/v1/user/repos", map[string]any{"name": "personal", "auto_init": true}, nil)
}

// post sends an authenticated JSON POST and requires a 2xx response,
// decoding the body into out when it's non-nil.
func (f forgejoFixture) post(t *testing.T, path string, body, out any) {
	t.Helper()
	f.do(t, http.MethodPost, path, body, out)
}

// patch sends an authenticated JSON PATCH and requires a 2xx response.
func (f forgejoFixture) patch(t *testing.T, path string, body any) {
	t.Helper()
	f.do(t, http.MethodPatch, path, body, nil)
}

// get sends an authenticated GET and decodes the response body into out.
func (f forgejoFixture) get(t *testing.T, path string, out any) {
	t.Helper()
	f.do(t, http.MethodGet, path, nil, out)
}

func (f forgejoFixture) do(t *testing.T, method, path string, body, out any) {
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
