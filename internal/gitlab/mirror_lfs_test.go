//go:build integration && gitlab

package gitlab_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
	"github.com/alrayyes/backup-git-repos/internal/httpauth"
	"github.com/stretchr/testify/require"
)

// TestMirrorSyncFetchesLFSContent proves issue #92's first acceptance
// criterion against a real GitLab CE instance: a project with an
// LFS-tracked file mirrors with the actual object content present, not
// just the pointer, so git lfs fsck against the resulting mirror reports
// no missing objects. GitLab CE ships Git LFS on by default at both the
// instance and project level -- confirmed live rather than assumed, since
// omnibusConfig in container_test.go turns off every other optional
// subsystem this package's tests don't need and LFS was never one of
// them, and this test needs no extra toggle to pass.
// t.Parallel() deliberately stays off on all three tests in this file:
// each boots its own real GitLab CE container (startWithLFS), and running
// them concurrently would mean several full GitLab CE instances at once on
// whatever's running the nightly lane -- a fixed, predictable resource cost
// matters more here than the wall-clock time saved, the same "shared...
// fixed external resource" exception rules/go-test.md carves out.
func TestMirrorSyncFetchesLFSContent(t *testing.T) {
	f := startWithLFS(t)
	f.pushLFSRepo(t, "lfs-repo")

	client, err := gitlab.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	remote := client.Remote(backup.Repo{Path: "team/lfs-repo"})
	dir := filepath.Join(t.TempDir(), "lfs-repo.git")
	m := backup.Mirror{}

	require.NoError(t, m.Sync(context.Background(), remote, dir))

	requireLFSFsckOK(t, dir)
}

// TestMirrorSyncFetchesLFSContentOnUpdate proves the second acceptance
// criterion: a mirror produced by a run before this feature landed -- a
// plain "git clone --mirror" with no LFS objects fetched at all -- gets
// its LFS content filled in by a later Sync, through the incremental
// update path rather than a full re-clone.
func TestMirrorSyncFetchesLFSContentOnUpdate(t *testing.T) {
	f := startWithLFS(t)
	cloneURL := f.pushLFSRepo(t, "lfs-repo-update")

	client, err := gitlab.New(f.BaseURL, f.Token)
	require.NoError(t, err)
	remote := client.Remote(backup.Repo{Path: "team/lfs-repo-update"})
	dir := filepath.Join(t.TempDir(), "lfs-repo-update.git")

	legacyMirrorClone(t, dir, cloneURL, f.Token)
	require.NoDirExists(t, filepath.Join(dir, "lfs"), "the legacy clone should carry no lfs objects yet")

	m := backup.Mirror{}
	require.NoError(t, m.Sync(context.Background(), remote, dir))

	requireLFSFsckOK(t, dir)
}

// TestMirrorSyncSkipsLFSForOrdinaryRepo proves that a project which never
// used LFS mirrors exactly as it did before this feature, with no lfs/
// directory created at all -- proof that mirroring an ordinary project
// triggers no LFS call. It uses the package's ordinary start() fixture
// rather than startWithLFS: an ordinary project has nothing to prove
// about GitLab's own LFS server, and start()'s dynamic port keeps this
// test as cheap as every other one in the package.
func TestMirrorSyncSkipsLFSForOrdinaryRepo(t *testing.T) {
	f := start(t)
	client, err := gitlab.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	remote := client.Remote(backup.Repo{Path: "team/active-repo"})
	dir := filepath.Join(t.TempDir(), "active-repo.git")
	m := backup.Mirror{}

	require.NoError(t, m.Sync(context.Background(), remote, dir))

	require.NoDirExists(t, filepath.Join(dir, "lfs"))
}

// startWithLFS boots its own GitLab CE container rather than reusing the
// package's shared start(), and fixes the container's host port and its
// external_url to the same, pre-chosen value ahead of time. Git LFS's
// batch API embeds an absolute href built from external_url -- unlike
// ordinary git-http and the REST API, which start()'s dynamically
// assigned port already proves respond relative to whatever host the
// client actually connects to -- so a random host port would leave the
// LFS batch response naming a different address than the one the
// fixture's own push, and the mirror clone under test, actually connect
// to. The same gotcha startWithLFS in internal/forgejo already worked
// around for Forgejo's ROOT_URL.
func startWithLFS(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()

	hostPort := freeTCPPort(t)
	baseURL := "http://localhost:" + hostPort

	ctr := runGitLabConfigured(ctx, t, baseURL, hostPort)

	mintToken(ctx, t, ctr)

	f := fixture{BaseURL: baseURL, Token: rootToken}
	f.seed(t)

	return f
}

// freeTCPPort picks a currently unused TCP port so startWithLFS can fix
// GitLab's external_url and its container's host port binding to the same
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

// pushLFSRepo creates a project under the "team" group already seeded by
// fixture.seed and pushes a single LFS-tracked file into it over
// git-http, authenticated the same oauth2-Basic header Client.Remote
// itself hands Mirror for the clone step. It returns the project's clone
// URL.
func (f fixture) pushLFSRepo(t *testing.T, name string) string {
	t.Helper()

	var group struct {
		ID int `json:"id"`
	}
	f.do(t, http.MethodGet, "/api/v4/groups/team", nil, &group)
	f.post(t, "/api/v4/projects", map[string]any{"name": name, "namespace_id": group.ID}, nil)

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
	plainGit(t, dir, "config", "http.extraHeader", "Authorization: Basic "+httpauth.Basic("oauth2", f.Token))
	plainGit(t, dir, "config", "user.email", "test@example.com")
	plainGit(t, dir, "config", "user.name", "test")
	plainGit(t, dir, "lfs", "install", "--local")
	plainGit(t, dir, "lfs", "track", "*.bin")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.bin"), []byte("real lfs content, not a pointer\n"), 0o600))
	plainGit(t, dir, "add", ".gitattributes", "big.bin")
	plainGit(t, dir, "commit", "-m", "add lfs file")
	plainGit(t, dir, "push", "origin", "HEAD:main")

	return cloneURL
}

// legacyMirrorClone fabricates the mirror a run made before this feature
// landed: a plain "git clone --mirror" with no LFS fetch at all.
func legacyMirrorClone(t *testing.T, dir, cloneURL, token string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(dir), 0o750))
	runAuthedGit(t, filepath.Dir(dir), token, "clone", "--mirror", cloneURL, dir)
}

// plainGit runs git in dir with no credentials -- local commands that
// never talk to the remote (config, lfs track, add, commit).
func plainGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

// runAuthedGit authenticates the same way Mirror itself does: the token
// as an oauth2-user Basic Authorization header, scoped to the remote and
// passed through the environment, never the command line or a persisted
// git config.
func runAuthedGit(t *testing.T, dir, token string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.extraHeader",
		"GIT_CONFIG_VALUE_0=Authorization: Basic "+httpauth.Basic("oauth2", token),
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func requireLFSFsckOK(t *testing.T, dir string) {
	t.Helper()
	out := gitOutput(t, dir, "lfs", "fsck")
	require.Contains(t, out, "Git LFS fsck OK")
}
