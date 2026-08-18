package backup

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The paths every Lister's and Runner's fixture data is expected to seed.
// An adapter's own fixture-seeding driver (the fake, Forgejo, GitLab) is
// responsible for creating repositories under these exact paths, which is
// what lets the suites below run unchanged against any of them.
const (
	TestActiveRepoPath   = "team/active-repo"
	TestArchivedRepoPath = "team/archived-repo"
	TestEmptyRepoPath    = "team/empty-repo"
)

// TestLister runs the behaviour every Lister must satisfy, whether it's
// backed by a fake or a real forge. Exported here, rather than living in a
// _test.go file, so an adapter under internal/ can run the same suite
// against itself -- otherwise an unexported helper in this package's own
// tests would be unreachable from anywhere that isn't this package.
func TestLister(t *testing.T, newLister func(t *testing.T) Lister) {
	t.Helper()

	t.Run("lists active repos", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), StateActive)
		require.NoError(t, err)

		paths := repoPaths(repos)
		require.Contains(t, paths, TestActiveRepoPath)
		require.NotContains(t, paths, TestArchivedRepoPath)
	})

	t.Run("lists archived repos", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), StateArchived)
		require.NoError(t, err)

		paths := repoPaths(repos)
		require.Contains(t, paths, TestArchivedRepoPath)
		require.NotContains(t, paths, TestActiveRepoPath)
	})

	t.Run("lists all repos", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), StateAll)
		require.NoError(t, err)

		paths := repoPaths(repos)
		require.Contains(t, paths, TestActiveRepoPath)
		require.Contains(t, paths, TestArchivedRepoPath)
	})

	t.Run("carries the full namespace path", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), StateAll)
		require.NoError(t, err)

		require.Equal(t, TestActiveRepoPath, findRepo(t, repos, TestActiveRepoPath).Path)
	})

	t.Run("reports empty repos", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), StateAll)
		require.NoError(t, err)

		require.True(t, findRepo(t, repos, TestEmptyRepoPath).Empty)
	})
}

func repoPaths(repos []Repo) []string {
	paths := make([]string, len(repos))
	for i, r := range repos {
		paths[i] = r.Path
	}
	return paths
}

func findRepo(t *testing.T, repos []Repo, path string) Repo {
	t.Helper()
	for _, r := range repos {
		if r.Path == path {
			return r
		}
	}
	t.Fatalf("no repo with path %q in %v", path, repoPaths(repos))
	return Repo{}
}

// TestDriver runs a backup the way Runner.Run does, whether that's against
// an in-memory fake or a real forge container.
type TestDriver func(ctx context.Context, opts Options) (Result, error)

// TestBackup runs the specification every forge's backup pipeline must
// satisfy, in domain terms: it talks only about a destination directory and
// the state it asks for, so the same spec runs unchanged against a fake or
// a real forge.
func TestBackup(t *testing.T, run TestDriver) {
	t.Helper()
	t.Run("backs up active repos", func(t *testing.T) { backsUpActiveRepos(t, run) })
	t.Run("keeps the forge folder structure", func(t *testing.T) { keepsTheForgeFolderStructure(t, run) })
	t.Run("leaves archived repos alone when active only", func(t *testing.T) { leavesArchivedReposAloneWhenActiveOnly(t, run) })
	t.Run("skips empty repos", func(t *testing.T) { skipsEmptyRepos(t, run) })
	t.Run("archives only the selected repos", func(t *testing.T) { archivesOnlyTheSelectedRepos(t, run) })
	t.Run("does not persist a mirror for an archived repo that gets archived", func(t *testing.T) {
		doesNotPersistMirrorForArchivedRepo(t, run)
	})
}

func backsUpActiveRepos(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()

	_, err := run(context.Background(), Options{Dest: dest, State: StateActive})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dest, TestActiveRepoPath+".git", "HEAD"))
}

func keepsTheForgeFolderStructure(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()

	_, err := run(context.Background(), Options{Dest: dest, State: StateAll})
	require.NoError(t, err)

	require.DirExists(t, filepath.Join(dest, "team"))
	require.FileExists(t, filepath.Join(dest, TestActiveRepoPath+".git", "HEAD"))
}

func leavesArchivedReposAloneWhenActiveOnly(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()

	_, err := run(context.Background(), Options{Dest: dest, State: StateActive})
	require.NoError(t, err)

	require.NoFileExists(t, filepath.Join(dest, TestArchivedRepoPath+".git", "HEAD"))
}

func archivesOnlyTheSelectedRepos(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()
	archiveDir := t.TempDir()

	result, err := run(context.Background(), Options{
		Dest:       dest,
		State:      StateAll,
		Archive:    ArchiveArchived,
		ArchiveDir: archiveDir,
	})
	require.NoError(t, err)

	require.Equal(t, 1, result.Archived)
	require.FileExists(t, filepath.Join(archiveDir, TestArchivedRepoPath+".tar.gz"))
	require.NoFileExists(t, filepath.Join(archiveDir, TestActiveRepoPath+".tar.gz"))
}

func doesNotPersistMirrorForArchivedRepo(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()
	archiveDir := t.TempDir()

	_, err := run(context.Background(), Options{
		Dest:       dest,
		State:      StateAll,
		Archive:    ArchiveAll,
		ArchiveDir: archiveDir,
	})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(archiveDir, TestArchivedRepoPath+".tar.gz"))
	require.NoDirExists(t, filepath.Join(dest, TestArchivedRepoPath+".git"))

	// An active repo still gets both -- it's still being written to, so it
	// still needs an incrementally updated mirror, not just a snapshot.
	require.FileExists(t, filepath.Join(dest, TestActiveRepoPath+".git", "HEAD"))
	require.FileExists(t, filepath.Join(archiveDir, TestActiveRepoPath+".tar.gz"))
}

func skipsEmptyRepos(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()

	result, err := run(context.Background(), Options{Dest: dest, State: StateAll})
	require.NoError(t, err)

	require.NoDirExists(t, filepath.Join(dest, TestEmptyRepoPath+".git"))
	require.GreaterOrEqual(t, result.Skipped, 1)
}
