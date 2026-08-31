package backup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// exampleConfig is what `config init` writes: a starter file with an
// example forge commented out, since its token is only a placeholder to
// replace with a real one before it's worth uncommenting.
const exampleConfig = `# backup-git-repos config. See:
# https://github.com/alrayyes/backup-git-repos#configuration
dest: /srv/backups/git

forges: []
# forges:
#   - name: work # becomes the top-level folder for this forge's repos
#     kind: gitlab # forgejo, gitlab, or github
#     url: https://gitlab.example.com # omit for github
#     token: paste-your-token-here
`

func newConfigCommand(flags *cliFlags) *cobra.Command {
	var force bool

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage the backup-git-repos config file",
	}

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Write a starter config file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigInit(cmd, *flags, force)
		},
	}
	initCmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config file")

	configCmd.AddCommand(initCmd)
	return configCmd
}

func runConfigInit(cmd *cobra.Command, flags cliFlags, force bool) error {
	path, err := configInitPath(flags)
	if err != nil {
		return err
	}

	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config already exists at %s (use --force to overwrite)", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	// 0o750: the file written into it can carry a literal token, so the
	// directory itself starts unreadable by anyone but the owner and group
	// too, not just the file.
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	// 0o600: a config file can carry a literal token (see the `token` field
	// in Config), so it starts private rather than readable by anyone who
	// can already read the rest of $XDG_CONFIG_HOME.
	if err := os.WriteFile(path, []byte(exampleConfig), 0o600); err != nil {
		return err
	}

	cmd.Println("wrote config to " + path)
	return nil
}

// promptConfigInit asks whether to write a starter config to path, reads a
// single line off cmd.InOrStdin(), and runs the same write runConfigInit's
// own command does on anything read as yes. Declining, or reading nothing
// at all (an immediately closed stdin), counts as no -- this is a [y/N]
// prompt, not a [Y/n] one.
func promptConfigInit(cmd *cobra.Command, flags cliFlags, path string) (bool, error) {
	cmd.Println("no config file found at " + path)
	cmd.Print("run `config init` now? [y/N] ")

	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read answer: %w", err)
	}
	answer := strings.TrimSpace(line)
	if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
		return false, nil
	}

	if err := runConfigInit(cmd, flags, false); err != nil {
		return false, err
	}
	return true, nil
}

// configInitPath is where `config init` writes: --config/-c if given,
// otherwise defaultConfigPath.
func configInitPath(flags cliFlags) (string, error) {
	if flags.config != "" {
		return expandHome(flags.config)
	}
	return defaultConfigPath()
}

// defaultConfigPath is $XDG_CONFIG_HOME/backup-git-repos/config.yaml
// (os.UserConfigDir already falls back to ~/.config when XDG_CONFIG_HOME
// isn't set) -- what `config init` writes by default, and what `run` and
// `list` fall back to reading when --config isn't passed.
func defaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}
	return filepath.Join(dir, "backup-git-repos", "config.yaml"), nil
}
