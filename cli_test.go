package backup_test

import (
	"bytes"
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
