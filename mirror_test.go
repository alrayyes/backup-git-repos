//go:build integration

package backup_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

func TestMirrorSync(t *testing.T) {
	fixture := startForgejo(t)
	remote := backup.Remote{
		CloneURL:   fixture.BaseURL + "/team/active-repo.git",
		AuthHeader: "Basic " + base64.StdEncoding.EncodeToString([]byte(fixture.AdminUsername+":"+fixture.Token)),
	}
	dir := filepath.Join(t.TempDir(), "active-repo.git")
	m := backup.Mirror{}
	ctx := context.Background()

	t.Run("creates a bare mirror on the first run", func(t *testing.T) {
		require.NoError(t, m.Sync(ctx, remote, dir))
		require.FileExists(t, filepath.Join(dir, "HEAD"))
	})

	t.Run("keeps every branch and tag", func(t *testing.T) {
		refs := gitOutput(t, dir, "show-ref")
		require.Contains(t, refs, "refs/heads/feature")
		require.Contains(t, refs, "refs/tags/v1.0.0")
	})

	t.Run("fetches new commits on a second run", func(t *testing.T) {
		fixture.post(t, "/api/v1/repos/team/active-repo/contents/new.txt", map[string]any{
			"content": base64.StdEncoding.EncodeToString([]byte("hi")),
			"message": "add new.txt",
			"branch":  "main",
		}, nil)

		require.NoError(t, m.Sync(ctx, remote, dir))

		require.Contains(t, gitOutput(t, dir, "log", "--oneline", "main"), "add new.txt")
	})

	t.Run("prunes a branch deleted upstream", func(t *testing.T) {
		fixture.do(t, http.MethodDelete, "/api/v1/repos/team/active-repo/branches/feature", nil, nil)

		require.NoError(t, m.Sync(ctx, remote, dir))

		require.NotContains(t, gitOutput(t, dir, "show-ref"), "refs/heads/feature")
	})

	t.Run("never writes the token into git config", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(dir, "config"))
		require.NoError(t, err)
		require.NotContains(t, string(data), fixture.Token)
	})
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}
