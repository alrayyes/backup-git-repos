package backup_test

import (
	"context"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

// driver runs a backup the way backup.Runner.Run does, whether that's
// against an in-memory fake or a real forge container. The specification
// below talks only in these terms -- a destination directory and the state
// it asks for -- so the same spec runs unchanged against either.
type driver func(ctx context.Context, opts backup.Options) (backup.Result, error)

func testBackup(t *testing.T, run driver) {
	t.Helper()
	t.Run("backs up active repos", func(t *testing.T) { backsUpActiveRepos(t, run) })
	t.Run("keeps the forge folder structure", func(t *testing.T) { keepsTheForgeFolderStructure(t, run) })
	t.Run("leaves archived repos alone when active only", func(t *testing.T) { leavesArchivedReposAloneWhenActiveOnly(t, run) })
	t.Run("skips empty repos", func(t *testing.T) { skipsEmptyRepos(t, run) })
}

func backsUpActiveRepos(t *testing.T, run driver) {
	t.Helper()
	dest := t.TempDir()

	_, err := run(context.Background(), backup.Options{Dest: dest, State: backup.StateActive})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dest, activeRepoPath+".git", "HEAD"))
}

func keepsTheForgeFolderStructure(t *testing.T, run driver) {
	t.Helper()
	dest := t.TempDir()

	_, err := run(context.Background(), backup.Options{Dest: dest, State: backup.StateAll})
	require.NoError(t, err)

	require.DirExists(t, filepath.Join(dest, "team"))
	require.FileExists(t, filepath.Join(dest, activeRepoPath+".git", "HEAD"))
}

func leavesArchivedReposAloneWhenActiveOnly(t *testing.T, run driver) {
	t.Helper()
	dest := t.TempDir()

	_, err := run(context.Background(), backup.Options{Dest: dest, State: backup.StateActive})
	require.NoError(t, err)

	require.NoFileExists(t, filepath.Join(dest, archivedRepoPath+".git", "HEAD"))
}

func skipsEmptyRepos(t *testing.T, run driver) {
	t.Helper()
	dest := t.TempDir()

	result, err := run(context.Background(), backup.Options{Dest: dest, State: backup.StateAll})
	require.NoError(t, err)

	require.NoDirExists(t, filepath.Join(dest, emptyRepoPath+".git"))
	require.GreaterOrEqual(t, result.Skipped, 1)
}
