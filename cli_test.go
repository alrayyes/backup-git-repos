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

func TestCLIHelpListsCommands(t *testing.T) {
	root := backup.NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})

	require.NoError(t, root.Execute())

	require.Contains(t, out.String(), "run")
	require.Contains(t, out.String(), "list")
}

func TestCLIRunRequiresDest(t *testing.T) {
	path := writeConfig(t, configWithNoDest)

	root := backup.NewRootCommand("test")
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", path})

	err := root.Execute()

	require.ErrorContains(t, err, "--dest")
}

func TestCLIRunRejectsUnknownState(t *testing.T) {
	root := backup.NewRootCommand("test")
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", "config.yaml", "--dest", "/tmp/dest", "--state", "bogus"})

	err := root.Execute()

	require.ErrorIs(t, err, backup.ErrBadState)
}
