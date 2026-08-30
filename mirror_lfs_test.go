package backup_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

// missingGitLFS points Mirror.GitLFSPath at a path that can never resolve,
// so a test can prove git-lfs was never even looked for -- if Sync
// succeeds, LFS was skipped; if it fails, it's ErrGitLFSNotFound.
const missingGitLFS = "/nonexistent/git-lfs"

// TestMirrorSyncLFS proves LFS detection needs nothing but git itself: a
// repository that never used LFS mirrors fine even with git-lfs entirely
// unreachable, on both a first clone and a later refresh, while a
// repository that does use LFS fails fast and clearly instead of the
// clone silently missing the file contents. Fetching the LFS content
// itself, once git-lfs is actually available, is proven against a real
// forge in internal/forgejo's integration suite -- this one only needs
// git, so it stays in the fast lane.
func TestMirrorSyncLFS(t *testing.T) {
	t.Parallel()

	t.Run("skips lfs entirely for a repo that doesn't use it", func(t *testing.T) {
		t.Parallel()
		origin := lfsOriginRepo(t, false)
		dir := filepath.Join(t.TempDir(), "mirror.git")
		m := backup.Mirror{GitLFSPath: missingGitLFS}

		require.NoError(t, m.Sync(context.Background(), backup.Remote{CloneURL: origin}, dir))
	})

	t.Run("skips lfs on a refresh too, for a repo that doesn't use it", func(t *testing.T) {
		t.Parallel()
		origin := lfsOriginRepo(t, false)
		dir := filepath.Join(t.TempDir(), "mirror.git")
		m := backup.Mirror{GitLFSPath: missingGitLFS}
		remote := backup.Remote{CloneURL: origin}

		require.NoError(t, m.Sync(context.Background(), remote, dir))
		require.NoError(t, m.Sync(context.Background(), remote, dir))
	})
}

// TestMirrorSyncDetectsLFSTrackedInANestedGitattributes proves usesLFS
// isn't blind to a repository that sets up LFS tracking per-directory
// rather than with a single root .gitattributes, which git lfs track
// itself writes by default but which a monorepo can equally override.
func TestMirrorSyncDetectsLFSTrackedInANestedGitattributes(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	runGit(t, src, "init", "-q", "-b", "main")
	require.NoError(t, os.Mkdir(filepath.Join(src, "assets"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(src, "assets", ".gitattributes"),
		[]byte("*.bin filter=lfs diff=lfs merge=lfs -text\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(src, "assets", "big.bin"),
		[]byte("stand-in for real lfs pointer content\n"), 0o600))
	runGit(t, src, "add", "assets")
	runGit(t, src, "commit", "-q", "-m", "add lfs file under a subdirectory")

	originDir := t.TempDir()
	origin := filepath.Join(originDir, "repo.git")
	runGit(t, src, "clone", "-q", "--bare", src, origin)

	dir := filepath.Join(t.TempDir(), "mirror.git")
	m := backup.Mirror{GitLFSPath: missingGitLFS}

	err := m.Sync(context.Background(), backup.Remote{CloneURL: origin}, dir)

	// GitLFSPath is set to a path that doesn't resolve via ErrGitLFSNotFound
	// (that sentinel only comes from an empty GitLFSPath falling through to
	// exec.LookPath) but Sync still has to try to run it and fail, which is
	// exactly the proof that usesLFS recognized the nested .gitattributes:
	// against a repo that doesn't use LFS at all, the same bogus path is
	// never reached and Sync succeeds, per the subtests above.
	require.ErrorContains(t, err, missingGitLFS,
		"a repo with only a nested .gitattributes should still be detected as using LFS")
}

// TestMirrorSyncFailsFastWhenLFSNeededButMissing is its own top-level test,
// not a subtest of TestMirrorSyncLFS, because it needs t.Setenv to point
// PATH at a directory with git but no git-lfs -- and t.Setenv panics once
// the test or any ancestor has called t.Parallel(), which TestMirrorSyncLFS
// itself does.
func TestMirrorSyncFailsFastWhenLFSNeededButMissing(t *testing.T) {
	origin := lfsOriginRepo(t, true)
	dir := filepath.Join(t.TempDir(), "mirror.git")
	t.Setenv("PATH", pathWithGitButNoGitLFS(t))
	m := backup.Mirror{}

	err := m.Sync(context.Background(), backup.Remote{CloneURL: origin}, dir)

	require.ErrorIs(t, err, backup.ErrGitLFSNotFound)
}

// pathWithGitButNoGitLFS returns a PATH entry that resolves "git" but not
// "git-lfs", regardless of whether the two happen to live in the same
// directory on this machine (as they typically do on a package-managed
// system): a fresh directory holding nothing but a symlink to the real git
// binary. Git's own subcommands live under its exec-path rather than PATH,
// so this doesn't break git itself, only the separate git-lfs binary.
func pathWithGitButNoGitLFS(t *testing.T) string {
	t.Helper()

	git, err := exec.LookPath("git")
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.Symlink(git, filepath.Join(dir, "git")))

	return dir
}

// lfsOriginRepo creates a bare git repository seeded with one commit. With
// tracksLFS set, that commit also carries a .gitattributes naming the lfs
// filter and a plain-text stand-in for the tracked file -- fabricated with
// plain git rather than the real git-lfs CLI, since detection only ever
// inspects .gitattributes and never needs git-lfs to run at all.
func lfsOriginRepo(t *testing.T, tracksLFS bool) string {
	t.Helper()

	src := t.TempDir()
	runGit(t, src, "init", "-q", "-b", "main")

	if tracksLFS {
		require.NoError(t, os.WriteFile(filepath.Join(src, ".gitattributes"),
			[]byte("*.bin filter=lfs diff=lfs merge=lfs -text\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(src, "big.bin"),
			[]byte("stand-in for real lfs pointer content\n"), 0o600))
		runGit(t, src, "add", ".gitattributes", "big.bin")
		runGit(t, src, "commit", "-q", "-m", "add lfs file")
	} else {
		runGit(t, src, "commit", "-q", "--allow-empty", "-m", "initial commit")
	}

	dir := t.TempDir()
	bare := filepath.Join(dir, "repo.git")
	runGit(t, src, "clone", "-q", "--bare", src, bare)

	return bare
}
