package forgejo_test

import (
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/stretchr/testify/require"
)

func TestRemoteWithSSHKey(t *testing.T) {
	client, err := forgejo.New("https://git.example.org", "unused-token")
	require.NoError(t, err)
	client.SSHKey = &backup.SSHKey{Path: "/home/me/.ssh/deploy_key"}

	t.Run("clones over ssh on the standard port by default", func(t *testing.T) {
		remote := client.Remote(backup.Repo{Path: "team/repo"})
		require.Equal(t, "ssh://git@git.example.org/team/repo.git", remote.CloneURL)
		require.Equal(t, client.SSHKey, remote.SSHKey)
		require.Empty(t, remote.AuthHeader)
	})

	t.Run("SSHHost overrides the default host and port", func(t *testing.T) {
		client.SSHHost = "git.example.org:2222"
		defer func() { client.SSHHost = "" }()

		remote := client.Remote(backup.Repo{Path: "team/repo"})
		require.Equal(t, "ssh://git@git.example.org:2222/team/repo.git", remote.CloneURL)
	})
}
