package backup

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"time"

	"github.com/spf13/cobra"
)

// ErrBadState means --state wasn't one of all, active, or archived.
var ErrBadState = errors.New("state must be one of: all, active, archived")

// ParseState parses a --state flag value.
func ParseState(s string) (State, error) {
	switch s {
	case "", "all":
		return StateAll, nil
	case "active":
		return StateActive, nil
	case "archived":
		return StateArchived, nil
	default:
		return 0, fmt.Errorf("%w: got %q", ErrBadState, s)
	}
}

// ErrBadArchive means --archive wasn't one of none, all, active, or archived.
var ErrBadArchive = errors.New("archive must be one of: none, all, active, archived")

// ParseArchive parses a --archive flag value.
func ParseArchive(s string) (ArchiveSelection, error) {
	switch s {
	case "", "none":
		return ArchiveNone, nil
	case "all":
		return ArchiveAll, nil
	case "active":
		return ArchiveActive, nil
	case "archived":
		return ArchiveArchived, nil
	default:
		return 0, fmt.Errorf("%w: got %q", ErrBadArchive, s)
	}
}

type cliFlags struct {
	config      string
	dest        string
	forges      []string
	state       string
	archive     string
	archiveDir  string
	concurrency int
	timeout     time.Duration
}

// NewRunner builds the Runner for a configured forge. Implementations live
// with their adapters under internal/, so this package -- which every
// adapter imports for Repo, Lister and the rest -- can't reference them
// directly without a cycle. The composition root (main) supplies one.
type NewRunner func(ForgeConfig) (Runner, error)

// NewRootCommand builds the backup-git-repos command line. Each RunE stays a
// thin shell: parse and validate the flags, then call into runBackup, which
// knows nothing about cobra.
func NewRootCommand(version string, newRunner NewRunner) *cobra.Command {
	var flags cliFlags

	root := &cobra.Command{
		Use:           "backup-git-repos",
		Short:         "Back up git repositories from GitLab, Forgejo, and GitHub",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVarP(&flags.config, "config", "c", "", "config file (required)")

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Mirror repositories from the configured forges",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBackup(cmd, flags, newRunner, false)
		},
	}
	addRunFlags(runCmd, &flags)
	runCmd.Flags().StringVarP(&flags.dest, "dest", "d", "", "backup destination directory (required, or set dest in the config)")
	runCmd.Flags().StringVar(&flags.archive, "archive", "none", "also write out repositories as tar.gz: none, all, active, or archived")
	runCmd.Flags().StringVar(&flags.archiveDir, "archive-dir", "", "where archives go (default: <dest>/archive)")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Print what would be backed up, clone nothing",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBackup(cmd, flags, newRunner, true)
		},
	}
	addRunFlags(listCmd, &flags)

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(version)
			return nil
		},
	}

	root.AddCommand(runCmd, listCmd, versionCmd)
	return root
}

func addRunFlags(cmd *cobra.Command, flags *cliFlags) {
	cmd.Flags().StringArrayVar(&flags.forges, "forge", nil, "restrict to these named forges (repeatable)")
	cmd.Flags().StringVar(&flags.state, "state", "all", "all, active, or archived")
	cmd.Flags().IntVarP(&flags.concurrency, "concurrency", "j", 0, "repositories mirrored in parallel (default: number of CPUs)")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 30*time.Minute, "per-repository timeout")
}

func runBackup(cmd *cobra.Command, flags cliFlags, newRunner NewRunner, listOnly bool) error {
	if flags.config == "" {
		return errors.New("--config is required")
	}
	state, err := ParseState(flags.state)
	if err != nil {
		return err
	}
	archive, err := ParseArchive(flags.archive)
	if err != nil {
		return err
	}

	cfg, err := LoadConfig(flags.config)
	if err != nil {
		return err
	}

	dest := flags.dest
	if dest == "" {
		dest = cfg.Dest
	}
	if dest == "" && !listOnly {
		return errors.New("--dest is required, or set dest in the config file")
	}

	archiveDir := flags.archiveDir
	if archiveDir == "" {
		archiveDir = filepath.Join(dest, "archive")
	}

	log := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil))

	for _, fc := range cfg.Forges {
		if len(flags.forges) > 0 && !slices.Contains(flags.forges, fc.Name) {
			continue
		}

		if err := runForge(cmd, fc, dest, archiveDir, state, archive, flags, newRunner, listOnly, log); err != nil {
			return err
		}
	}

	return nil
}

func runForge(
	cmd *cobra.Command, fc ForgeConfig, dest, archiveDir string, state State, archive ArchiveSelection,
	flags cliFlags, newRunner NewRunner, listOnly bool, log *slog.Logger,
) error {
	runner, err := newRunner(fc)
	if err != nil {
		return err
	}

	if listOnly {
		repos, err := runner.Lister.ListRepos(cmd.Context(), state)
		if err != nil {
			return fmt.Errorf("list %s: %w", fc.Name, err)
		}
		for _, r := range repos {
			cmd.Println(fc.Name + "/" + r.Path)
		}
		return nil
	}

	result, err := runner.Run(cmd.Context(), Options{
		Dest:        filepath.Join(dest, fc.Name),
		State:       state,
		Archive:     archive,
		ArchiveDir:  filepath.Join(archiveDir, fc.Name),
		Concurrency: flags.concurrency,
		Timeout:     flags.timeout,
		Log:         log,
	})
	if err != nil {
		return fmt.Errorf("run %s: %w", fc.Name, err)
	}
	cmd.Printf("%s: synced %d, skipped %d, failed %d, archived %d\n",
		fc.Name, result.Synced, result.Skipped, result.Failed, result.Archived)
	return nil
}
