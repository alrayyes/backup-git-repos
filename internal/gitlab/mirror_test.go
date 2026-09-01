//go:build integration && gitlab

package gitlab_test

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
	"github.com/stretchr/testify/require"
)

// TestMirrorSync exercises backup.Mirror -- forge-agnostic, already proven
// against Forgejo -- against a real GitLab project. It's the same type with
// zero changes; only the Remote it's handed differs.
// No t.Parallel() here: start(t) boots its own real GitLab CE container,
// and this package's container-booting tests running concurrently would
// mean several full instances at once on whatever's running the nightly
// lane -- see mirror_lfs_test.go's own tests for the same reasoning.
func TestMirrorSync(t *testing.T) {
	f := start(t)
	client, err := gitlab.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	remote := client.Remote(backup.Repo{Path: "team/active-repo"})
	dir := filepath.Join(t.TempDir(), "active-repo.git")
	m := backup.Mirror{}
	ctx := context.Background()

	// These subtests are not independent: each mutates the same dir/remote
	// state the previous one left behind (a second Sync after a commit, a
	// third after a branch deletion) -- a real exception, not left off by
	// default. None of them gets t.Parallel().
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
		f.post(t, "/api/v4/projects/team%2Factive-repo/repository/files/new.txt", map[string]any{
			"branch":         "main",
			"content":        "hi",
			"commit_message": "add new.txt",
		}, nil)

		require.NoError(t, m.Sync(ctx, remote, dir))

		require.Contains(t, gitOutput(t, dir, "log", "--oneline", "main"), "add new.txt")
	})

	t.Run("prunes a branch deleted upstream", func(t *testing.T) {
		f.do(t, http.MethodDelete, "/api/v4/projects/team%2Factive-repo/repository/branches/feature", nil, nil)

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
