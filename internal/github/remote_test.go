package github_test

import (
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/github"
	"github.com/stretchr/testify/require"
)

func TestRemote(t *testing.T) {
	client, err := github.New("", "gh-token")
	require.NoError(t, err)

	remote := client.Remote(backup.Repo{Path: "team/active-repo"})

	t.Run("clones from github.com, not the api base", func(t *testing.T) {
		require.Equal(t, "https://github.com/team/active-repo.git", remote.CloneURL)
	})

	t.Run("carries the token as the basic auth username", func(t *testing.T) {
		require.Equal(t, "Basic Z2gtdG9rZW46", remote.AuthHeader)
	})
}

func TestRemoteWithSSHKey(t *testing.T) {
	client, err := github.New("", "gh-token")
	require.NoError(t, err)
	client.SSHKey = &backup.SSHKey{Path: "/home/me/.ssh/deploy_key"}

	remote := client.Remote(backup.Repo{Path: "team/active-repo"})

	t.Run("clones over ssh as the git user", func(t *testing.T) {
		require.Equal(t, "ssh://git@github.com/team/active-repo.git", remote.CloneURL)
	})

	t.Run("carries the key instead of a token", func(t *testing.T) {
		require.Equal(t, client.SSHKey, remote.SSHKey)
		require.Empty(t, remote.AuthHeader)
	})
}
