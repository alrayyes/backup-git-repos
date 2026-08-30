//go:build integration

package forgejo_test

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
	tcforgejo "github.com/testcontainers/testcontainers-go/modules/forgejo"
)

// fixture is a running Forgejo instance seeded with a known set of
// repositories: an active one carrying an extra branch and tag, an archived
// one, an empty one, and a personal one under the admin account.
type fixture struct {
	BaseURL       string
	Token         string
	AdminUsername string

	// SSHHost is the container's own mapped SSH endpoint (host:port) --
	// never the standard port 22, which testcontainers maps to something
	// else at random, the same reason ConnectionString above can't be
	// trusted for the HTTP port either. Client.SSHHost in mirror_ssh_test.go
	// points at this instead of assuming the default.
	SSHHost string
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

	sshHost, err := ctr.SSHConnectionString(ctx)
	require.NoError(t, err)

	f := fixture{
		BaseURL:       baseURL,
		Token:         mintToken(ctx, t, ctr),
		AdminUsername: ctr.AdminUsername(),
		SSHHost:       sshHost,
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
func (f fixture) seed(t *testing.T) {
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
func (f fixture) post(t *testing.T, path string, body, out any) {
	t.Helper()
	f.do(t, http.MethodPost, path, body, out)
}

// addSSHKey adds an authorized-keys-format public key to the admin
// account -- the same thing a real user does to clone with a deploy key --
// so mirror_ssh_test.go can mirror against this instance with the matching
// private key and no token at all.
func (f fixture) addSSHKey(t *testing.T, title, publicKey string) {
	t.Helper()
	f.post(t, "/api/v1/user/keys", map[string]any{"title": title, "key": publicKey}, nil)
}

// patch sends an authenticated JSON PATCH and requires a 2xx response.
func (f fixture) patch(t *testing.T, path string, body any) {
	t.Helper()
	f.do(t, http.MethodPatch, path, body, nil)
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
