package backup_test

import (
	"context"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

// The paths every Lister's fixture data is expected to seed. An adapter's
// own fixture-seeding driver (the fake, Forgejo, GitLab) is responsible for
// creating repositories under these exact paths, which is what lets a single
// contract suite run against all of them unchanged.
const (
	activeRepoPath   = "team/active-repo"
	archivedRepoPath = "team/archived-repo"
	emptyRepoPath    = "team/empty-repo"
)

// testLister runs the behaviour every Lister must satisfy, whether it's
// backed by a fake or a real forge.
func testLister(t *testing.T, newLister func(t *testing.T) backup.Lister) {
	t.Helper()

	t.Run("lists active repos", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), backup.StateActive)
		require.NoError(t, err)

		paths := repoPaths(repos)
		require.Contains(t, paths, activeRepoPath)
		require.NotContains(t, paths, archivedRepoPath)
	})

	t.Run("lists archived repos", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), backup.StateArchived)
		require.NoError(t, err)

		paths := repoPaths(repos)
		require.Contains(t, paths, archivedRepoPath)
		require.NotContains(t, paths, activeRepoPath)
	})

	t.Run("lists all repos", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), backup.StateAll)
		require.NoError(t, err)

		paths := repoPaths(repos)
		require.Contains(t, paths, activeRepoPath)
		require.Contains(t, paths, archivedRepoPath)
	})

	t.Run("carries the full namespace path", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), backup.StateAll)
		require.NoError(t, err)

		require.Equal(t, activeRepoPath, findRepo(t, repos, activeRepoPath).Path)
	})

	t.Run("reports empty repos", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), backup.StateAll)
		require.NoError(t, err)

		require.True(t, findRepo(t, repos, emptyRepoPath).Empty)
	})
}

func repoPaths(repos []backup.Repo) []string {
	paths := make([]string, len(repos))
	for i, r := range repos {
		paths[i] = r.Path
	}
	return paths
}

func findRepo(t *testing.T, repos []backup.Repo, path string) backup.Repo {
	t.Helper()
	for _, r := range repos {
		if r.Path == path {
			return r
		}
	}
	t.Fatalf("no repo with path %q in %v", path, repoPaths(repos))
	return backup.Repo{}
}
