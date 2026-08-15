package backup_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

// bareRepo creates a bare repo with one commit, the way a mirror clone
// would leave one on disk.
func bareRepo(t *testing.T) string {
	t.Helper()

	src := t.TempDir()
	runGit(t, src, "init", "-q", "-b", "main")
	runGit(t, src, "commit", "-q", "--allow-empty", "-m", "initial commit")

	dir := t.TempDir()
	bare := filepath.Join(dir, "repo.git")
	runGit(t, src, "clone", "-q", "--bare", src, bare)

	return bare
}

// runGit sets a commit identity via env vars rather than relying on the
// machine's global git config, which a CI runner has none of.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestArchiveWritesAGzippedTar(t *testing.T) {
	repo := bareRepo(t)
	out := filepath.Join(t.TempDir(), "repo.tar.gz")

	require.NoError(t, backup.Archive(repo, out))

	require.FileExists(t, out)
}

func TestArchiveExtractsToAClonableRepo(t *testing.T) {
	repo := bareRepo(t)
	out := filepath.Join(t.TempDir(), "repo.tar.gz")
	require.NoError(t, backup.Archive(repo, out))

	extractDir := t.TempDir()
	runTar(t, extractDir, "-xzf", out)

	restored := t.TempDir()
	runGit(t, restored, "clone", "-q", filepath.Join(extractDir, "repo.git"), ".")

	log := gitLog(t, restored)
	require.Contains(t, log, "initial commit")
}

func TestArchiveEntriesAreWrappedInAFolderNamedAfterTheMirror(t *testing.T) {
	repo := bareRepo(t)
	out := filepath.Join(t.TempDir(), "repo.tar.gz")
	require.NoError(t, backup.Archive(repo, out))

	extractDir := t.TempDir()
	runTar(t, extractDir, "-xzf", out)

	require.FileExists(t, filepath.Join(extractDir, "repo.git", "HEAD"))
}

func TestArchiveLeavesNoTmpFileOnSuccess(t *testing.T) {
	repo := bareRepo(t)
	out := filepath.Join(t.TempDir(), "repo.tar.gz")

	require.NoError(t, backup.Archive(repo, out))

	require.NoFileExists(t, out+".tmp")
}

func TestArchiveIsReproducible(t *testing.T) {
	repo := bareRepo(t)
	dir := t.TempDir()
	first := filepath.Join(dir, "first.tar.gz")
	second := filepath.Join(dir, "second.tar.gz")

	require.NoError(t, backup.Archive(repo, first))
	require.NoError(t, backup.Archive(repo, second))

	firstBytes, err := os.ReadFile(first)
	require.NoError(t, err)
	secondBytes, err := os.ReadFile(second)
	require.NoError(t, err)

	require.Equal(t, firstBytes, secondBytes)
}

func runTar(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("tar", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func gitLog(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "log", "--oneline")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}
