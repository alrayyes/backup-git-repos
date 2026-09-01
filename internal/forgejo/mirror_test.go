//go:build integration

package forgejo_test

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/stretchr/testify/require"
)

func TestMirrorSync(t *testing.T) {
	t.Parallel()

	f := start(t)
	client, err := forgejo.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	remote := client.Remote(backup.Repo{Path: "team/active-repo"})
	dir := filepath.Join(t.TempDir(), "active-repo.git")
	m := backup.Mirror{}
	ctx := context.Background()

	// Deliberately serial: each subtest below builds on the mirror state
	// (the same dir, synced against the same upstream) the previous one
	// left behind -- "fetches new commits on a second run" only means
	// anything once "creates a bare mirror on the first run" has actually
	// run first, the same reasoning applies down the rest of the chain, so
	// t.Parallel() on any of them would both race and be meaningless.
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
		f.post(t, "/api/v1/repos/team/active-repo/contents/new.txt", map[string]any{
			"content": "aGk=", // "hi"
			"message": "add new.txt",
			"branch":  "main",
		}, nil)

		require.NoError(t, m.Sync(ctx, remote, dir))

		require.Contains(t, gitOutput(t, dir, "log", "--oneline", "main"), "add new.txt")
	})

	t.Run("prunes a branch deleted upstream", func(t *testing.T) {
		f.do(t, http.MethodDelete, "/api/v1/repos/team/active-repo/branches/feature", nil, nil)

		require.NoError(t, m.Sync(ctx, remote, dir))

		require.NotContains(t, gitOutput(t, dir, "show-ref"), "refs/heads/feature")
	})

	t.Run("never writes the token into git config", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(dir, "config"))
		require.NoError(t, err)
		require.NotContains(t, string(data), f.Token)
	})
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	return string(out)
}
