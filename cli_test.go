package backup_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

// configWithNoDest is a syntactically valid config with no dest field and no
// forges, so LoadConfig succeeds and --dest's own validation is what's
// actually under test.
const configWithNoDest = "forges: []\n"

// neverNewRunner fails the test if called. None of the CLI tests here reach
// the point of actually running a backup -- they all exercise flag and
// config validation, which happens first -- so a real adapter factory would
// never be invoked either.
func neverNewRunner(t *testing.T) backup.NewRunner {
	return func(backup.ForgeConfig) (backup.Runner, error) {
		t.Fatal("newRunner should not have been called")
		return backup.Runner{}, nil
	}
}

func TestCLIHelpListsCommands(t *testing.T) {
	root := backup.NewRootCommand("test", neverNewRunner(t))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})

	require.NoError(t, root.Execute())

	require.Contains(t, out.String(), "run")
	require.Contains(t, out.String(), "list")
}

func TestCLIRunRequiresDest(t *testing.T) {
	path := writeConfig(t, configWithNoDest)

	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", path})

	err := root.Execute()

	require.ErrorContains(t, err, "--dest")
}

func TestCLIRunRejectsUnknownState(t *testing.T) {
	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", "config.yaml", "--dest", "/tmp/dest", "--state", "bogus"})

	err := root.Execute()

	require.ErrorIs(t, err, backup.ErrBadState)
}

func TestCLIRunRejectsUnknownArchive(t *testing.T) {
	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", "config.yaml", "--dest", "/tmp/dest", "--archive", "bogus"})

	err := root.Execute()

	require.ErrorIs(t, err, backup.ErrBadArchive)
}

// dirCapturingMirrorer records every dir it was asked to sync into, without
// touching disk -- these tests care only about the path, not the mirror
// contents.
type dirCapturingMirrorer struct {
	dirs *[]string
}

func (m dirCapturingMirrorer) Sync(_ context.Context, _ backup.Remote, dir string) error {
	*m.dirs = append(*m.dirs, dir)
	return nil
}

func newRunnerCapturingDirs(dirs *[]string) backup.NewRunner {
	return func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{
			Lister:   newFakeLister(),
			Mirrorer: dirCapturingMirrorer{dirs: dirs},
			Remoter:  fakeRemoter{},
		}, nil
	}
}

func TestCLIRunExpandsTildeInDestFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TEST_TILDE_TOKEN", "secret")
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_TILDE_TOKEN\n")

	var dirs []string
	root := backup.NewRootCommand("test", newRunnerCapturingDirs(&dirs))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", "~/backups"})

	require.NoError(t, root.Execute())

	require.NotEmpty(t, dirs)
	for _, dir := range dirs {
		require.True(t, strings.HasPrefix(dir, home), "dir %q should be under expanded home %q", dir, home)
	}
}

func TestCLIRunExpandsTildeInDestFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TEST_TILDE_TOKEN", "secret")
	cfgPath := writeConfig(t, "dest: ~/backups\nforges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_TILDE_TOKEN\n")

	var dirs []string
	root := backup.NewRootCommand("test", newRunnerCapturingDirs(&dirs))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", cfgPath})

	require.NoError(t, root.Execute())

	require.NotEmpty(t, dirs)
	for _, dir := range dirs {
		require.True(t, strings.HasPrefix(dir, home), "dir %q should be under expanded home %q", dir, home)
	}
}

func TestCLIRunExpandsTildeInArchiveDirFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TEST_TILDE_TOKEN", "secret")
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_TILDE_TOKEN\n")

	newRunner := func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{}}, nil
	}
	root := backup.NewRootCommand("test", newRunner)
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{
		"run", "--config", cfgPath, "--dest", t.TempDir(),
		"--archive", "all", "--archive-dir", "~/archives",
	})

	require.NoError(t, root.Execute())

	require.FileExists(t, filepath.Join(home, "archives", "home", backup.TestActiveRepoPath+".tar.gz"))
}

func TestCLIRunLeavesOtherUserTildePathUnexpanded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TEST_TILDE_TOKEN", "secret")
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_TILDE_TOKEN\n")

	var dirs []string
	root := backup.NewRootCommand("test", newRunnerCapturingDirs(&dirs))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", "~otheruser/backups"})

	require.NoError(t, root.Execute())

	require.NotEmpty(t, dirs)
	for _, dir := range dirs {
		require.Contains(t, dir, "~otheruser/backups")
	}
}
