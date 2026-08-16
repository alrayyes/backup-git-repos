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
