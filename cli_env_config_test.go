package backup_test

import (
	"bytes"
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

// TestCLIRunDestFromEnvironmentVariableAlone proves the environment-variable
// layer works with no --dest flag and no dest in the config file --
// rules/cli.md's flag > environment variable > config file > default
// precedence, exercised at its simplest level.
func TestCLIRunDestFromEnvironmentVariableAlone(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_ENV_DEST_TOKEN\n")
	t.Setenv("TEST_ENV_DEST_TOKEN", "secret")

	dest := t.TempDir()
	t.Setenv("BACKUP_GIT_REPOS_DEST", dest)

	var dirs []string
	root := backup.NewRootCommand("test", newRunnerCapturingDirs(&dirs))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", cfgPath, "--state", "active"})

	require.NoError(t, root.Execute())

	require.Equal(t, []string{filepath.Join(dest, "home", backup.TestActiveRepoPath+".git")}, dirs)
}

// TestCLIRunDestFlagOverridesEnvironmentVariable proves --dest wins when
// both it and BACKUP_GIT_REPOS_DEST are set to different values.
func TestCLIRunDestFlagOverridesEnvironmentVariable(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_ENV_DEST_TOKEN\n")
	t.Setenv("TEST_ENV_DEST_TOKEN", "secret")

	t.Setenv("BACKUP_GIT_REPOS_DEST", t.TempDir())
	flagDest := t.TempDir()

	var dirs []string
	root := backup.NewRootCommand("test", newRunnerCapturingDirs(&dirs))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", flagDest, "--state", "active"})

	require.NoError(t, root.Execute())

	require.Equal(t, []string{filepath.Join(flagDest, "home", backup.TestActiveRepoPath+".git")}, dirs)
}

// TestCLIRunDestEnvironmentVariableOverridesConfigFile proves
// BACKUP_GIT_REPOS_DEST wins over the config file's own dest when --dest
// isn't passed.
func TestCLIRunDestEnvironmentVariableOverridesConfigFile(t *testing.T) {
	cfgPath := writeConfig(t, "dest: /this-config-dest-should-lose\nforges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_ENV_DEST_TOKEN\n")
	t.Setenv("TEST_ENV_DEST_TOKEN", "secret")

	envDest := t.TempDir()
	t.Setenv("BACKUP_GIT_REPOS_DEST", envDest)

	var dirs []string
	root := backup.NewRootCommand("test", newRunnerCapturingDirs(&dirs))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", cfgPath, "--state", "active"})

	require.NoError(t, root.Execute())

	require.Equal(t, []string{filepath.Join(envDest, "home", backup.TestActiveRepoPath+".git")}, dirs)
}

// TestCLIRunArchiveDirEnvironmentVariableUsesMultiWordName proves a
// multi-word flag maps to its environment variable predictably:
// --archive-dir becomes BACKUP_GIT_REPOS_ARCHIVE_DIR, the flag's name
// upper-cased with dashes replaced by underscores.
func TestCLIRunArchiveDirEnvironmentVariableUsesMultiWordName(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_ENV_ARCHIVEDIR_TOKEN\n")
	t.Setenv("TEST_ENV_ARCHIVEDIR_TOKEN", "secret")

	archiveDir := t.TempDir()
	t.Setenv("BACKUP_GIT_REPOS_ARCHIVE_DIR", archiveDir)

	newRunner := func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{}}, nil
	}
	root := backup.NewRootCommand("test", newRunner)
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", t.TempDir(), "--archive", "all"})

	require.NoError(t, root.Execute())

	require.FileExists(t, filepath.Join(archiveDir, "home", backup.TestActiveRepoPath+".tar.gz"))
}

// TestCLIRunForgeEnvironmentVariableRestrictsForges proves
// BACKUP_GIT_REPOS_FORGE restricts a run to the named forge with no --forge
// flag, the same way --forge already does.
func TestCLIRunForgeEnvironmentVariableRestrictsForges(t *testing.T) {
	cfgPath := writeConfig(t, `
forges:
  - name: a
    kind: forgejo
    url: https://git.example.org
    token_env: TEST_ENV_FORGE_A
  - name: b
    kind: forgejo
    url: https://git.example.org
    token_env: TEST_ENV_FORGE_B
`)
	t.Setenv("TEST_ENV_FORGE_A", "secret")
	t.Setenv("TEST_ENV_FORGE_B", "secret")
	t.Setenv("BACKUP_GIT_REPOS_FORGE", "a")

	var dirs []string
	newRunner := func(fc backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{
			Lister:   perForgeLister{forge: fc.Name},
			Mirrorer: newDirCapturingMirrorer(&dirs),
			Remoter:  fakeRemoter{},
		}, nil
	}

	dest := t.TempDir()
	root := backup.NewRootCommand("test", newRunner)
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", dest})

	require.NoError(t, root.Execute())

	require.Equal(t, []string{filepath.Join(dest, "a", "a-repo.git")}, dirs)
}

// TestCLIRunConcurrencyFlagWinsOverEnvironmentVariableEvenAtZero proves an
// explicitly-passed --concurrency 0 (meaning "default to NumCPU", per its
// own help text) still wins over BACKUP_GIT_REPOS_CONCURRENCY, rather than
// zero being mistaken for "unset" and falling through to the environment
// variable -- the ambiguity design.md flagged as this change's main risk.
// Proven by actually counting how many repositories Run starts
// concurrently, the same barrierMirrorer/nRepoLister approach
// TestRunDefaultsConcurrencyToNumCPU (run_test.go) already uses.
func TestCLIRunConcurrencyFlagWinsOverEnvironmentVariableEvenAtZero(t *testing.T) {
	n := runtime.GOMAXPROCS(0)
	if n < 2 {
		t.Skip("GOMAXPROCS is 1 on this host; can't distinguish concurrency 1 from NumCPU")
	}

	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_ENV_CONCURRENCY_TOKEN\n")
	t.Setenv("TEST_ENV_CONCURRENCY_TOKEN", "secret")
	// Deliberately not n: if the bug this test guards against existed
	// (--concurrency 0 mistaken for "unset" and falling through to the
	// environment variable instead of resolving to NumCPU), only 1
	// repository would ever reach the barrier below and the test would
	// time out waiting for the rest.
	t.Setenv("BACKUP_GIT_REPOS_CONCURRENCY", "1")

	arrived := make(chan struct{}, n)
	release := make(chan struct{})
	newRunner := func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{
			Lister:   nRepoLister{n: n},
			Mirrorer: barrierMirrorer{arrived: arrived, release: release},
			Remoter:  fakeRemoter{},
		}, nil
	}

	root := backup.NewRootCommand("test", newRunner)
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", t.TempDir(), "--concurrency", "0"})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root.SetContext(ctx)

	done := make(chan error, 1)
	go func() { done <- root.Execute() }()

	// All n repos reaching the barrier proves --concurrency 0 resolved to
	// NumCPU (n), not to BACKUP_GIT_REPOS_CONCURRENCY's own value of 1 --
	// which is exactly what 0 mistaken for "unset" would have fallen
	// through to, and only the first repo would ever reach this loop.
	for range n {
		select {
		case <-arrived:
		case <-ctx.Done():
			t.Fatal("timed out waiting for repos to start concurrently")
		}
	}
	close(release)
	require.NoError(t, <-done)
}
