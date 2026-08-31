package backup_test

import (
	"bytes"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

func TestCLIRunVerbosePrintsDebugLines(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_VERBOSE_TOKEN\n")
	t.Setenv("TEST_VERBOSE_TOKEN", "secret")

	newRunner := func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{}}, nil
	}
	root := backup.NewRootCommand("test", newRunner)
	var stderr bytes.Buffer
	root.SetOut(new(bytes.Buffer))
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", t.TempDir(), "--verbose"})

	require.NoError(t, root.Execute())

	require.Contains(t, stderr.String(), "mirroring")
}

func TestCLIRunWithoutVerboseOmitsDebugLines(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_VERBOSE_TOKEN\n")
	t.Setenv("TEST_VERBOSE_TOKEN", "secret")

	newRunner := func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{}}, nil
	}
	root := backup.NewRootCommand("test", newRunner)
	var stderr bytes.Buffer
	root.SetOut(new(bytes.Buffer))
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", t.TempDir()})

	require.NoError(t, root.Execute())

	require.NotContains(t, stderr.String(), "mirroring")
}

func TestCLIRunWritesNoProgressBarWhenNotATerminal(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_VERBOSE_TOKEN\n")
	t.Setenv("TEST_VERBOSE_TOKEN", "secret")

	newRunner := func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{}}, nil
	}
	root := backup.NewRootCommand("test", newRunner)
	var stderr bytes.Buffer
	root.SetOut(new(bytes.Buffer))
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", t.TempDir()})

	require.NoError(t, root.Execute())

	require.NotContains(t, stderr.String(), "\r", "no carriage-return-redrawn progress bar expected when stderr isn't a terminal")
}
