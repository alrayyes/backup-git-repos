package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// exampleConfig is what `config init` writes: a starter file with an
// example forge commented out, since it names a token_env the user hasn't
// necessarily set yet, and LoadConfig fails startup on that.
const exampleConfig = `# backup-git-repos config. See:
# https://github.com/alrayyes/backup-git-repos#configuration
dest: /srv/backups/git

forges: []
# forges:
#   - name: work # becomes the top-level folder for this forge's repos
#     kind: gitlab # forgejo, gitlab, or github
#     url: https://gitlab.example.com # omit for github
#     token_env: WORK_GITLAB_TOKEN
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

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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
