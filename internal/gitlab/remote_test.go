package gitlab_test

import (
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
	"github.com/stretchr/testify/require"
)

func TestRemoteWithSSHKey(t *testing.T) {
	client, err := gitlab.New("https://gitlab.example.com", "unused-token")
	require.NoError(t, err)
	client.SSHKey = &backup.SSHKey{Path: "/home/me/.ssh/deploy_key"}

	t.Run("clones over ssh on the standard port by default", func(t *testing.T) {
		remote := client.Remote(backup.Repo{Path: "group/subgroup/repo"})
		require.Equal(t, "ssh://git@gitlab.example.com/group/subgroup/repo.git", remote.CloneURL)
		require.Equal(t, client.SSHKey, remote.SSHKey)
		require.Empty(t, remote.AuthHeader)
	})

	t.Run("SSHHost overrides the default host and port", func(t *testing.T) {
		client.SSHHost = "gitlab.example.com:2222"
		defer func() { client.SSHHost = "" }()

		remote := client.Remote(backup.Repo{Path: "group/repo"})
		require.Equal(t, "ssh://git@gitlab.example.com:2222/group/repo.git", remote.CloneURL)
	})
}
