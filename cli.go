package backup

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	verbose     bool
	dryRun      bool
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
	root.PersistentFlags().StringVarP(&flags.config, "config", "c", "",
		"config file (default: $XDG_CONFIG_HOME/backup-git-repos/config.yaml)")

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
	runCmd.Flags().BoolVarP(&flags.verbose, "verbose", "v", false, "print per-repository progress as it happens")
	runCmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "print what a run would do, without cloning or writing anything")

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

	root.AddCommand(runCmd, listCmd, versionCmd, newConfigCommand(&flags))
	return root
}

func addRunFlags(cmd *cobra.Command, flags *cliFlags) {
	cmd.Flags().StringArrayVar(&flags.forges, "forge", nil, "restrict to these named forges (repeatable)")
	cmd.Flags().StringVar(&flags.state, "state", "all", "all, active, or archived")
	cmd.Flags().IntVarP(&flags.concurrency, "concurrency", "j", 0, "repositories mirrored in parallel (default: number of CPUs)")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 30*time.Minute, "per-repository timeout")
}

func runBackup(cmd *cobra.Command, flags cliFlags, newRunner NewRunner, listOnly bool) error {
	configPath, err := resolveConfigPath(flags.config)
	if err != nil {
		return err
	}
	state, err := ParseState(flags.state)
	if err != nil {
		return err
	}
	archive, err := ParseArchive(flags.archive)
	if err != nil {
		return err
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}

	dest, archiveDir, err := resolveDestPaths(flags, cfg, listOnly)
	if err != nil {
		return err
	}

	log := newCLILogger(cmd.ErrOrStderr(), flags.verbose)

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

// resolveConfigPath returns explicit, expanded, if --config was passed,
// otherwise falls back to defaultConfigPath -- but only when a file
// actually exists there, so a bare `run` doesn't silently pick up
// whatever's sitting in $XDG_CONFIG_HOME when the user never asked for it.
func resolveConfigPath(explicit string) (string, error) {
	if explicit != "" {
		return expandHome(explicit)
	}

	path, err := defaultConfigPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("--config is required: no config file at the default path %s", path)
	}
	return path, nil
}

// resolveDestPaths applies the --dest/config-file fallback and --archive-dir's
// default of <dest>/archive, expanding a leading "~" in either.
func resolveDestPaths(flags cliFlags, cfg Config, listOnly bool) (dest, archiveDir string, err error) {
	dest = flags.dest
	if dest == "" {
		dest = cfg.Dest
	}
	if dest == "" && !listOnly {
		return "", "", errors.New("--dest is required, or set dest in the config file")
	}
	dest, err = expandHome(dest)
	if err != nil {
		return "", "", err
	}

	archiveDir = flags.archiveDir
	if archiveDir == "" {
		archiveDir = filepath.Join(dest, "archive")
		return dest, archiveDir, nil
	}
	archiveDir, err = expandHome(archiveDir)
	if err != nil {
		return "", "", err
	}
	return dest, archiveDir, nil
}

// newCLILogger builds the Logger a run reports through: --verbose lowers
// the level so Run's per-repository slog.Debug lines reach w, otherwise
// only warnings, failures and the final summary do.
func newCLILogger(w io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

// expandHome resolves a leading "~" to the current user's home directory,
// the way a shell would before the tool ever sees the argument. It only
// resolves the current user's own "~" and "~/..." -- "~otheruser/..." passes
// through untouched, since resolving another account's home isn't something
// os.UserHomeDir can even answer.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand ~: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
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

	if flags.dryRun {
		return dryRunForge(cmd, fc, dest, state, archive, runner)
	}

	stderr := cmd.ErrOrStderr()
	result, err := runner.Run(cmd.Context(), Options{
		Dest:        filepath.Join(dest, fc.Name),
		State:       state,
		Archive:     archive,
		ArchiveDir:  filepath.Join(archiveDir, fc.Name),
		Concurrency: flags.concurrency,
		Timeout:     flags.timeout,
		Log:         log,
		Progress:    newProgressReporter(stderr, fc.Name, isTerminalWriter(stderr)),
	})
	if err != nil {
		return fmt.Errorf("run %s: %w", fc.Name, err)
	}
	cmd.Printf("%s: synced %d, skipped %d, failed %d, archived %d\n",
		fc.Name, result.Synced, result.Skipped, result.Failed, result.Archived)
	return nil
}

// dryRunForge previews what a real run would do to fc's repositories,
// without cloning, updating, or archiving anything: for each repository it
// prints the action a real run would take -- clone, update, or skip for an
// empty repo -- based only on whether a mirror already exists at its
// destination on disk, plus whether it would also be archived. The
// destination's own directory tree is read, never written.
func dryRunForge(cmd *cobra.Command, fc ForgeConfig, dest string, state State, archive ArchiveSelection, runner Runner) error {
	repos, err := runner.Lister.ListRepos(cmd.Context(), state)
	if err != nil {
		return fmt.Errorf("list %s: %w", fc.Name, err)
	}

	var synced, skipped, archived int
	for _, r := range repos {
		if r.Empty {
			cmd.Printf("%s/%s: skip (empty)\n", fc.Name, r.Path)
			skipped++
			continue
		}

		wantArchive := archive.wants(r.Archived)
		line := fc.Name + "/" + r.Path + ": " + dryRunAction(dest, fc.Name, r, wantArchive)
		if wantArchive {
			line += ", archive"
			archived++
		}
		cmd.Println(line)
		synced++
	}

	cmd.Printf("%s: would sync %d, skip %d, archive %d (dry run)\n", fc.Name, synced, skipped, archived)
	return nil
}

// dryRunAction reports "clone" or "update" the same way Mirror.Sync itself
// decides between them -- except an archived repository selected for
// archiving always mirrors into a fresh scratch directory in a real run, so
// it's always "clone" here too, regardless of what's already at dest.
func dryRunAction(dest, forge string, r Repo, wantArchive bool) string {
	if r.Archived && wantArchive {
		return "clone"
	}

	dir := filepath.Join(dest, forge, filepath.FromSlash(r.Path)+".git")
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err == nil {
		return "update"
	}
	return "clone"
}
