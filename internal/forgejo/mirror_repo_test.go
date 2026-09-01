//go:build integration

package forgejo_test

import (
	"context"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/stretchr/testify/require"
)

// migrateMirror creates team/mirror-repo as a mirror of team/active-repo,
// pointed at the instance's own internal address -- reachable from inside
// the container the same way any other clone_addr would be, but with no
// dependency on the outside network a real external upstream would need.
func (f fixture) migrateMirror(t *testing.T) {
	t.Helper()
	f.post(t, "/api/v1/repos/migrate", map[string]any{
		"clone_addr": "http://localhost:3000/team/active-repo.git",
		"repo_owner": "team",
		"repo_name":  "mirror-repo",
		"mirror":     true,
		"service":    "git",
		"auth_token": f.Token,
	}, nil)
}

func TestSkipMirrorsExcludesForgejoMirrorRepos(t *testing.T) {
	t.Parallel()

	f := start(t)
	f.migrateMirror(t)

	client, err := forgejo.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	// Both subtests toggle the same client's own SkipMirrors field, so they
	// stay serial rather than getting their own t.Parallel(): running them
	// concurrently would race one subtest's read against the other's
	// write/defer-restore of that shared field.
	t.Run("includes mirror repos by default", func(t *testing.T) {
		repos, err := client.ListRepos(context.Background(), backup.StateAll)
		require.NoError(t, err)
		require.Contains(t, repoPaths(t, repos), "team/mirror-repo")
	})

	t.Run("excludes mirror repos once SkipMirrors is set", func(t *testing.T) {
		client.SkipMirrors = true
		defer func() { client.SkipMirrors = false }()

		repos, err := client.ListRepos(context.Background(), backup.StateAll)
		require.NoError(t, err)

		paths := repoPaths(t, repos)
		require.NotContains(t, paths, "team/mirror-repo")
		require.Contains(t, paths, backup.TestActiveRepoPath)
	})
}

func repoPaths(t *testing.T, repos []backup.Repo) []string {
	t.Helper()
	paths := make([]string, len(repos))
	for i, r := range repos {
		paths[i] = r.Path
	}

	return paths
}
