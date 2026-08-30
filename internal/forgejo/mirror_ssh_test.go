//go:build integration

package forgejo_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// TestMirrorSyncOverSSH proves issue #9's first acceptance criterion
// against a real forge, not just the hand-rolled fixture server
// mirror_ssh_test.go in the root package exercises Mirror's own credential
// handling against: a repository configured with a deploy key -- no token
// on the Client at all -- mirrors over SSH through the whole real path,
// adapter included, exactly the way a real deploy-key user would run it.
func TestMirrorSyncOverSSH(t *testing.T) {
	f := start(t)
	keyPath := addDeployKey(t, f)

	client, err := forgejo.New(f.BaseURL, "")
	require.NoError(t, err)
	client.SSHKey = &backup.SSHKey{Path: keyPath}
	client.SSHHost = f.SSHHost

	remote := client.Remote(backup.Repo{Path: "team/active-repo"})
	dir := filepath.Join(t.TempDir(), "active-repo.git")
	m := backup.Mirror{}
	ctx := context.Background()

	t.Run("clones on the first run, no token involved", func(t *testing.T) {
		require.NoError(t, m.Sync(ctx, remote, dir))
		require.FileExists(t, filepath.Join(dir, "HEAD"))
	})

	t.Run("updates on a second run", func(t *testing.T) {
		require.NoError(t, m.Sync(ctx, remote, dir))
		require.FileExists(t, filepath.Join(dir, "HEAD"))
	})
}

// addDeployKey generates an ed25519 keypair, registers the public half on
// f's admin account the way a real deploy key gets added, writes the
// private half to a temp file with the permissions ssh requires, and
// returns its path.
func addDeployKey(t *testing.T, f fixture) string {
	t.Helper()

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	sshPub, err := ssh.NewPublicKey(pubKey)
	require.NoError(t, err)
	f.addSSHKey(t, "backup-it", string(ssh.MarshalAuthorizedKey(sshPub)))

	block, err := ssh.MarshalPrivateKey(privKey, "")
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0o600))

	return path
}
