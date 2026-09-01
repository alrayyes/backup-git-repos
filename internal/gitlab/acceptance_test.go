//go:build integration && gitlab

package gitlab_test

import (
	"context"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
	"github.com/stretchr/testify/require"
)

// TestBackupWikisAndSnippets exercises issue #6's acceptance criteria
// end-to-end against a real GitLab container: a project's wiki and its
// snippets are each their own git repository, reachable over the same
// git-http protocol as the project itself, but never returned by
// GET /api/v4/projects -- this is what proves ListRepos discovers them
// anyway, mirrors each one under its own path, and grows no entry at all
// for a project that carries neither.
// No t.Parallel() here: start(t) boots its own real GitLab CE container,
// and this package's tests each doing that concurrently would mean several
// full GitLab CE instances at once on whatever's running the nightly lane
// -- the same "shared... fixed external resource" exception
// rules/go-test.md carves out, and the reason internal/gitlab/mirror_lfs_test.go's
// three tests stay serial too. Its own subtests below only read files
// start(t) already finished writing, so they stay parallel among themselves.
func TestBackupWikisAndSnippets(t *testing.T) {
	f := start(t)
	client, err := gitlab.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	dest := t.TempDir()
	runner := backup.Runner{Lister: client, Mirrorer: backup.Mirror{}, Remoter: client}
	_, err = runner.Run(context.Background(), backup.Options{Dest: dest, State: backup.StateAll})
	require.NoError(t, err)

	t.Run("mirrors the project wiki alongside the project", func(t *testing.T) {
		t.Parallel()

		require.FileExists(t, filepath.Join(dest, backup.TestActiveRepoPath+".git", "HEAD"))
		require.FileExists(t, filepath.Join(dest, backup.TestActiveRepoPath+".wiki.git", "HEAD"))
	})

	t.Run("mirrors the snippet under a path distinct from the project", func(t *testing.T) {
		t.Parallel()

		require.NotEqual(t, backup.TestActiveRepoPath, f.SnippetPath)
		require.FileExists(t, filepath.Join(dest, f.SnippetPath+".git", "HEAD"))
	})

	t.Run("creates no wiki entry for a project with no wiki content", func(t *testing.T) {
		t.Parallel()

		require.NoDirExists(t, filepath.Join(dest, backup.TestArchivedRepoPath+".wiki.git"))
		require.NoDirExists(t, filepath.Join(dest, backup.TestEmptyRepoPath+".wiki.git"))
	})

	t.Run("creates no snippet entry for a project with no snippets", func(t *testing.T) {
		t.Parallel()

		require.NoDirExists(t, filepath.Join(dest, backup.TestArchivedRepoPath, "snippets"))
		require.NoDirExists(t, filepath.Join(dest, backup.TestEmptyRepoPath, "snippets"))
	})
}
