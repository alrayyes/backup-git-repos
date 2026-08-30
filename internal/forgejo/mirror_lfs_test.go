//go:build integration

package forgejo_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/alrayyes/backup-git-repos/internal/httpauth"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcforgejo "github.com/testcontainers/testcontainers-go/modules/forgejo"
)

// TestMirrorSyncFetchesLFSContent proves the primary acceptance criterion
// against a real Forgejo instance: a repository with an LFS-tracked file
// mirrors with the actual object content present, not just the pointer, so
// git lfs fsck against the resulting mirror reports no missing objects.
func TestMirrorSyncFetchesLFSContent(t *testing.T) {
	f := startWithLFS(t)
	f.pushLFSRepo(t, "lfs-repo")

	client, err := forgejo.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	remote := client.Remote(backup.Repo{Path: "team/lfs-repo"})
	dir := filepath.Join(t.TempDir(), "lfs-repo.git")
	m := backup.Mirror{}

	require.NoError(t, m.Sync(context.Background(), remote, dir))

	requireLFSFsckOK(t, dir)
}

// TestMirrorSyncFetchesLFSContentOnUpdate proves the second acceptance
// criterion: a mirror produced by a run before this feature landed -- a
// plain "git clone --mirror" with no LFS objects fetched at all -- gets its
// LFS content filled in by a later Sync, through the incremental update
// path rather than a full re-clone.
func TestMirrorSyncFetchesLFSContentOnUpdate(t *testing.T) {
	f := startWithLFS(t)
	cloneURL := f.pushLFSRepo(t, "lfs-repo-update")

	client, err := forgejo.New(f.BaseURL, f.Token)
	require.NoError(t, err)
	remote := client.Remote(backup.Repo{Path: "team/lfs-repo-update"})
	dir := filepath.Join(t.TempDir(), "lfs-repo-update.git")

	legacyMirrorClone(t, dir, cloneURL, f.Token)
	require.NoDirExists(t, filepath.Join(dir, "lfs"), "the legacy clone should carry no lfs objects yet")

	m := backup.Mirror{}
	require.NoError(t, m.Sync(context.Background(), remote, dir))

	requireLFSFsckOK(t, dir)
}

// TestMirrorSyncSkipsLFSForOrdinaryRepo proves the third acceptance
// criterion against a real forge: a repository that never used LFS mirrors
// exactly as it did before this feature, with no lfs/ directory created at
// all -- proof that mirroring an ordinary repository triggers no LFS call.
// It uses the package's ordinary start() fixture rather than
// startWithLFS: an ordinary repository has nothing to prove about
// Forgejo's own LFS server, and start()'s dynamic port keeps this test as
// cheap as every other one in the package.
func TestMirrorSyncSkipsLFSForOrdinaryRepo(t *testing.T) {
	f := start(t)
	client, err := forgejo.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	remote := client.Remote(backup.Repo{Path: "team/active-repo"})
	dir := filepath.Join(t.TempDir(), "active-repo.git")
	m := backup.Mirror{}

	require.NoError(t, m.Sync(context.Background(), remote, dir))

	require.NoDirExists(t, filepath.Join(dir, "lfs"))
}

// startWithLFS boots its own Forgejo container rather than reusing the
// package's shared start(), and turns Forgejo's own LFS server on
// (LFS_START_SERVER defaults off). It also has to fix the container's host
// port ahead of time and tell Forgejo its ROOT_URL is exactly that: unlike
// the git clone URL and the REST API, which Client.Remote already works
// around by deriving the URL it actually uses rather than trusting the
// server's own report of it, the LFS batch API embeds an absolute upload
// href chosen entirely server-side, built from ROOT_URL -- confirmed live,
// not assumed, since Forgejo's default of "http://localhost:3000/" (its
// own internal listening address) doesn't match whatever host port Docker
// happened to map otherwise. Fixing both to the same, pre-chosen value is
// what makes the two agree, so the fixture's own push -- and the real
// mirror clone under test -- land on the same address.
func startWithLFS(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()

	hostPort := freeTCPPort(t)
	rootURL := "http://localhost:" + hostPort + "/"

	ctr, err := tcforgejo.Run(ctx, image,
		tcforgejo.WithConfig("migrations", "ALLOW_LOCALNETWORKS", "true"),
		tcforgejo.WithConfig("server", "LFS_START_SERVER", "true"),
		tcforgejo.WithConfig("server", "ROOT_URL", rootURL),
		testcontainers.WithHostConfigModifier(func(hc *container.HostConfig) {
			hc.PortBindings = network.PortMap{
				network.MustParsePort("3000/tcp"): {{HostPort: hostPort}},
			}
		}),
	)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	f := fixture{
		BaseURL:       "http://localhost:" + hostPort,
		Token:         mintToken(ctx, t, ctr),
		AdminUsername: ctr.AdminUsername(),
	}
	f.seed(t)

	return f
}

// freeTCPPort picks a currently unused TCP port so startWithLFS can fix
// Forgejo's ROOT_URL and its container's host port binding to the same
// value before the container exists. There's an inherent race between
// closing this listener and Docker binding the same port, the same one
// every fixed-host-port test setup accepts; short-lived and low-volume
// enough here not to matter in practice.
func freeTCPPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

// pushLFSRepo creates an org repository and pushes a single LFS-tracked
// file into it over git-http, authenticated the same Basic header
// Client.Remote itself hands Mirror for the clone step. It returns the
// repo's clone URL.
func (f fixture) pushLFSRepo(t *testing.T, name string) string {
	t.Helper()
	f.post(t, "/api/v1/orgs/team/repos", map[string]any{"name": name}, nil)

	cloneURL := f.BaseURL + "/team/" + name + ".git"
	dir := t.TempDir()

	runAuthedGit(t, dir, f.Token, "clone", cloneURL, ".")
	// git-lfs authenticates its own batch-API and object-transfer requests
	// separately from git's own smart-HTTP transport, so the persisted
	// header below matters even though runAuthedGit's clone above already
	// authenticated the same way: unlike git, which picks up runAuthedGit's
	// scoped GIT_CONFIG_KEY_0 environment override, git-lfs doesn't, so
	// what it actually sends when pushing depends on the clone's own
	// (throwaway) config. Fine here: this is fixture setup seeding a
	// disposable container, not the library code under test, which never
	// persists a token to disk.
	plainGit(t, dir, "config", "http.extraHeader", "Authorization: Basic "+httpauth.Basic(f.AdminUsername, f.Token))
	plainGit(t, dir, "config", "user.email", "test@example.com")
	plainGit(t, dir, "config", "user.name", "test")
	plainGit(t, dir, "lfs", "install", "--local")
	plainGit(t, dir, "lfs", "track", "*.bin")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.bin"), []byte("real lfs content, not a pointer\n"), 0o600))
	plainGit(t, dir, "add", ".gitattributes", "big.bin")
	plainGit(t, dir, "commit", "-m", "add lfs file")
	plainGit(t, dir, "branch", "-M", "main")
	plainGit(t, dir, "push", "origin", "main")

	return cloneURL
}

// legacyMirrorClone fabricates the mirror a run made before this feature
// landed: a plain "git clone --mirror" with no LFS fetch at all.
func legacyMirrorClone(t *testing.T, dir, cloneURL, token string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(dir), 0o750))
	runAuthedGit(t, filepath.Dir(dir), token, "clone", "--mirror", cloneURL, dir)
}

// plainGit runs git in dir with no credentials -- local commands that never
// talk to the remote (config, lfs track, add, commit, branch).
func plainGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

// runAuthedGit authenticates the same way Mirror itself does: the token as
// a Basic Authorization header, scoped to the remote and passed through the
// environment, never the command line or a persisted git config.
func runAuthedGit(t *testing.T, dir, token string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.extraHeader",
		"GIT_CONFIG_VALUE_0=Authorization: Basic "+httpauth.Basic("backup-git-repos", token),
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

func requireLFSFsckOK(t *testing.T, dir string) {
	t.Helper()
	out := gitOutput(t, dir, "lfs", "fsck")
	require.Contains(t, out, "Git LFS fsck OK")
}
