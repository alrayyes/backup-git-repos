package backup_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// sshFixture is a minimal git-over-ssh server, good enough to exercise
// Mirror.Sync's SSH credential handling against a real ssh and git binary
// end to end without a container: it accepts exactly one public key and,
// for any "exec" request on a session channel, runs git-upload-pack
// against the one bare repo it seeds -- the same thing a real forge's SSH
// endpoint does for a mirror clone, and nothing else.
type sshFixture struct {
	// CloneURL is the seeded repo's ssh:// URL, ready to hand to
	// backup.Remote.
	CloneURL string
}

// startSSHFixture boots the server on an ephemeral localhost port and
// returns once it's accepting connections.
func startSSHFixture(t *testing.T, authorized ssh.PublicKey) sshFixture {
	t.Helper()

	repoPath := bareRepo(t)

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if string(key.Marshal()) != string(authorized.Marshal()) {
				return nil, errors.New("unauthorized key")
			}
			return nil, nil
		},
	}
	config.AddHostKey(newEd25519Signer(t))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	go acceptSSHConns(listener, config, repoPath)

	return sshFixture{CloneURL: "ssh://" + listener.Addr().String() + repoPath}
}

func acceptSSHConns(listener net.Listener, config *ssh.ServerConfig, repoPath string) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return // listener closed by the test's own cleanup
		}
		go serveSSHConn(conn, config, repoPath)
	}
}

func serveSSHConn(conn net.Conn, config *ssh.ServerConfig, repoPath string) {
	sconn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer func() { _ = sconn.Close() }()
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		channel, requests, err := newChan.Accept()
		if err != nil {
			continue
		}
		go serveSession(channel, requests, repoPath)
	}
}

// serveSession answers the single "exec" request a mirror clone or update
// sends -- git-upload-pack against some path -- by running git-upload-pack
// against this fixture's own seeded repo, regardless of the path the client
// actually asked for: there's only ever one repo to serve.
func serveSession(channel ssh.Channel, requests <-chan *ssh.Request, repoPath string) {
	defer func() { _ = channel.Close() }()

	for req := range requests {
		if req.Type != "exec" {
			_ = req.Reply(false, nil)
			continue
		}
		_ = req.Reply(true, nil)

		cmd := exec.Command("git-upload-pack", repoPath) //nolint:gosec // repoPath is this fixture's own seeded temp dir, never client input
		cmd.Stdin = channel
		cmd.Stdout = channel
		cmd.Stderr = channel.Stderr()

		status := uint32(0)
		if err := cmd.Run(); err != nil {
			status = 1
		}
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
		return
	}
}

func newEd25519Signer(t *testing.T) ssh.Signer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer
}

// writeSSHKey generates an ed25519 keypair, writes the private half to a
// temp file with the permissions ssh requires (world- or group-readable is
// refused outright), encrypted with passphrase when it's non-empty, and
// returns the file's path alongside the public half for the fixture
// server's PublicKeyCallback to check against.
func writeSSHKey(t *testing.T, passphrase string) (path string, pub ssh.PublicKey) {
	t.Helper()

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	sshPub, err := ssh.NewPublicKey(pubKey)
	require.NoError(t, err)

	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(privKey, "")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(privKey, "", []byte(passphrase))
	}
	require.NoError(t, err)

	path = filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0o600))

	return path, sshPub
}

// isolateSSHHome scopes known_hosts to a throwaway directory, rather than
// polluting whatever's running the test with entries for these ephemeral
// fixture servers -- the same reason it can't run alongside t.Parallel.
func isolateSSHHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestMirrorSyncOverSSH(t *testing.T) {
	isolateSSHHome(t)
	keyPath, pub := writeSSHKey(t, "")
	fixture := startSSHFixture(t, pub)

	remote := backup.Remote{CloneURL: fixture.CloneURL, SSHKey: &backup.SSHKey{Path: keyPath}}
	dir := filepath.Join(t.TempDir(), "repo.git")
	m := backup.Mirror{}

	t.Run("clones on the first run", func(t *testing.T) {
		require.NoError(t, m.Sync(context.Background(), remote, dir))
		require.FileExists(t, filepath.Join(dir, "HEAD"))
	})

	t.Run("updates on a second run", func(t *testing.T) {
		require.NoError(t, m.Sync(context.Background(), remote, dir))
		require.FileExists(t, filepath.Join(dir, "HEAD"))
	})

	t.Run("carries no Authorization credential", func(t *testing.T) {
		require.Empty(t, remote.AuthHeader)
	})
}

func TestMirrorSyncOverSSHWithPassphraseProtectedKey(t *testing.T) {
	isolateSSHHome(t)
	const passphrase = "s3cr3t-passphrase" //nolint:gosec // test fixture credential, not a real one
	keyPath, pub := writeSSHKey(t, passphrase)
	fixture := startSSHFixture(t, pub)

	remote := backup.Remote{
		CloneURL: fixture.CloneURL,
		SSHKey:   &backup.SSHKey{Path: keyPath, Passphrase: passphrase},
	}
	dir := filepath.Join(t.TempDir(), "repo.git")
	m := backup.Mirror{}

	start := time.Now()
	err := m.Sync(context.Background(), remote, dir)
	elapsed := time.Since(start)

	t.Run("clones without blocking on an interactive prompt", func(t *testing.T) {
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(dir, "HEAD"))
		require.Less(t, elapsed, 10*time.Second)
	})

	t.Run("never writes the passphrase into git config", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(dir, "config"))
		require.NoError(t, err)
		require.NotContains(t, string(data), passphrase)
	})
}

// TestMirrorSyncOverSSHFailsFastWithoutAPassphrase guards the never-block
// half of issue #9's third acceptance criterion from the other direction: a
// passphrase-protected key with no passphrase configured, and no agent to
// supply one, has to fail once BatchMode kicks in rather than hang waiting
// on a prompt nothing will ever answer.
func TestMirrorSyncOverSSHFailsFastWithoutAPassphrase(t *testing.T) {
	isolateSSHHome(t)
	t.Setenv("SSH_AUTH_SOCK", "") // no agent to satisfy the key from either
	keyPath, pub := writeSSHKey(t, "s3cr3t-passphrase")
	fixture := startSSHFixture(t, pub)

	remote := backup.Remote{CloneURL: fixture.CloneURL, SSHKey: &backup.SSHKey{Path: keyPath}}
	dir := filepath.Join(t.TempDir(), "repo.git")
	m := backup.Mirror{}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := m.Sync(ctx, remote, dir)

	require.Error(t, err)
	require.NotErrorIs(t, err, context.DeadlineExceeded, "must fail once BatchMode rejects the connection, not block until the timeout")
}
