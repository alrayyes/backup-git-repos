package backup_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

func TestCLIConfigInitWritesToXDGDefault(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg) // serial: t.Setenv panics under t.Parallel()

	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"config", "init"})

	require.NoError(t, root.Execute())

	path := filepath.Join(xdg, "backup-git-repos", "config.yaml")
	require.FileExists(t, path)

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(contents), "dest:")
	require.Contains(t, string(contents), "forges:")
}

func TestCLIConfigInitExampleUsesLiteralToken(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg) // serial: t.Setenv panics under t.Parallel()

	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"config", "init"})

	require.NoError(t, root.Execute())

	contents, err := os.ReadFile(filepath.Join(xdg, "backup-git-repos", "config.yaml"))
	require.NoError(t, err)

	require.Contains(t, string(contents), "token:")
	require.NotContains(t, string(contents), "token_env:")
}

func TestCLIConfigInitRespectsConfigFlag(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "custom.yaml")

	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"--config", path, "config", "init"})

	require.NoError(t, root.Execute())

	require.FileExists(t, path)
}

func TestCLIConfigInitRefusesToOverwriteWithoutForce(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("dest: /already/here\n"), 0o600))

	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"--config", path, "config", "init"})

	err := root.Execute()

	require.ErrorContains(t, err, path)
	contents, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "dest: /already/here\n", string(contents))
}

func TestCLIConfigInitOverwritesWithForce(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("dest: /already/here\n"), 0o600))

	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"--config", path, "config", "init", "--force"})

	require.NoError(t, root.Execute())

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(contents), "forges:")
	require.NotEqual(t, "dest: /already/here\n", string(contents))
}

func TestCLIRunOffersConfigInitWhenNoneFoundAndInteractive(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg) // serial: t.Setenv panics under t.Parallel()

	root := backup.NewRootCommand("test", neverNewRunner(t), backup.WithInteractive(true))
	root.SetOut(new(bytes.Buffer))
	root.SetIn(strings.NewReader("y\n"))
	root.SetArgs([]string{"run", "--dest", "/tmp/dest"})

	err := root.Execute()

	require.Error(t, err) // nothing to back up yet -- the point is to bootstrap, then stop.
	path := filepath.Join(xdg, "backup-git-repos", "config.yaml")
	require.FileExists(t, path)
}

func TestCLIRunConfigInitPromptDeclinedFallsBackToError(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg) // serial: t.Setenv panics under t.Parallel()

	root := backup.NewRootCommand("test", neverNewRunner(t), backup.WithInteractive(true))
	root.SetOut(new(bytes.Buffer))
	root.SetIn(strings.NewReader("n\n"))
	root.SetArgs([]string{"run", "--dest", "/tmp/dest"})

	err := root.Execute()

	require.ErrorContains(t, err, "--config is required")
	require.NoFileExists(t, filepath.Join(xdg, "backup-git-repos", "config.yaml"))
}

func TestCLIRunSkipsConfigInitPromptWithoutInteractive(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg) // serial: t.Setenv panics under t.Parallel()

	// No WithInteractive(true): defaults to real-terminal detection, which
	// a test process's stdin never satisfies -- exactly the case this
	// covers, without needing a real pty to prove it.
	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--dest", "/tmp/dest"})

	err := root.Execute()

	require.ErrorContains(t, err, "--config is required")
	require.NoFileExists(t, filepath.Join(xdg, "backup-git-repos", "config.yaml"))
}
