package backup_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestCLIRunFallsBackToDefaultConfigPath(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	dir := filepath.Join(xdg, "backup-git-repos")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configWithNoDest), 0o644))

	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--dest", t.TempDir()})

	require.NoError(t, root.Execute())
}

func TestCLIRunErrorsWhenNoConfigFlagAndNoDefaultFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--dest", t.TempDir()})

	err := root.Execute()

	require.ErrorContains(t, err, "--config")
	require.ErrorContains(t, err, filepath.Join("backup-git-repos", "config.yaml"))
}

func TestCLIRunPrefersExplicitConfigOverDefaultPath(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	require.NoError(t, os.MkdirAll(filepath.Join(xdg, "backup-git-repos"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(xdg, "backup-git-repos", "config.yaml"), []byte("dest: /wrong\nforges: []\n"), 0o644))

	explicit := writeConfig(t, configWithNoDest)

	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", explicit, "--dest", t.TempDir()})

	require.NoError(t, root.Execute())
}

// neverSyncMirrorer fails the test if Sync is ever called -- a dry run must
// never touch git or disk.
type neverSyncMirrorer struct{ t *testing.T }

func (m neverSyncMirrorer) Sync(context.Context, backup.Remote, string) error {
	m.t.Fatal("Sync should not have been called during a dry run")
	return nil
}

func newDryRunRunner(t *testing.T) backup.NewRunner {
	return func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{
			Lister:   newFakeLister(),
			Mirrorer: neverSyncMirrorer{t: t},
			Remoter:  fakeRemoter{},
		}, nil
	}
}

func TestCLIRunDryRunReportsCloneForNewRepos(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_DRYRUN_TOKEN\n")
	t.Setenv("TEST_DRYRUN_TOKEN", "secret")
	dest := t.TempDir()

	var out bytes.Buffer
	root := backup.NewRootCommand("test", newDryRunRunner(t))
	root.SetOut(&out)
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", dest, "--dry-run"})

	require.NoError(t, root.Execute())

	require.Contains(t, out.String(), "home/"+backup.TestActiveRepoPath+": clone")
	require.Contains(t, out.String(), "home/"+backup.TestEmptyRepoPath+": skip (empty)")
	require.NoDirExists(t, filepath.Join(dest, "home", backup.TestActiveRepoPath+".git"))
}

func TestCLIRunDryRunReportsUpdateForExistingMirror(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_DRYRUN_TOKEN\n")
	t.Setenv("TEST_DRYRUN_TOKEN", "secret")
	dest := t.TempDir()

	mirrorDir := filepath.Join(dest, "home", backup.TestActiveRepoPath+".git")
	require.NoError(t, os.MkdirAll(mirrorDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mirrorDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))

	var out bytes.Buffer
	root := backup.NewRootCommand("test", newDryRunRunner(t))
	root.SetOut(&out)
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", dest, "--dry-run"})

	require.NoError(t, root.Execute())

	require.Contains(t, out.String(), "home/"+backup.TestActiveRepoPath+": update")
}

func TestCLIRunDryRunFlagsArchiveSelectionWithoutWriting(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_DRYRUN_TOKEN\n")
	t.Setenv("TEST_DRYRUN_TOKEN", "secret")
	dest := t.TempDir()
	archiveDir := t.TempDir()

	var out bytes.Buffer
	root := backup.NewRootCommand("test", newDryRunRunner(t))
	root.SetOut(&out)
	root.SetArgs([]string{
		"run", "--config", cfgPath, "--dest", dest, "--dry-run",
		"--archive", "all", "--archive-dir", archiveDir,
	})

	require.NoError(t, root.Execute())

	require.Contains(t, out.String(), "home/"+backup.TestActiveRepoPath+": clone, archive")
	require.NoFileExists(t, filepath.Join(archiveDir, "home", backup.TestActiveRepoPath+".tar.gz"))
}

func TestCLIRunDryRunPrintsPreviewSummary(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_DRYRUN_TOKEN\n")
	t.Setenv("TEST_DRYRUN_TOKEN", "secret")
	dest := t.TempDir()

	var out bytes.Buffer
	root := backup.NewRootCommand("test", newDryRunRunner(t))
	root.SetOut(&out)
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", dest, "--dry-run", "--archive", "archived"})

	require.NoError(t, root.Execute())

	// newFakeLister seeds 2 non-empty repos (1 active, 1 archived) and 1
	// empty one; --archive archived selects just the archived one.
	require.Contains(t, out.String(), "home: would sync 2, skip 1, archive 1 (dry run)")
}

// logSettingLister records whatever logger SetLogger hands it, so a test
// can prove runForge actually wires one in rather than leaving an adapter
// to fall back to slog.Default() and log somewhere nobody's watching.
type logSettingLister struct {
	*fakeLister
	got *slog.Logger
}

func (l *logSettingLister) SetLogger(log *slog.Logger) { l.got = log }

func TestCLIRunSetsLoggerOnListerThatWantsOne(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_LOGSETTER_TOKEN\n")
	t.Setenv("TEST_LOGSETTER_TOKEN", "secret")

	lister := &logSettingLister{fakeLister: newFakeLister()}
	newRunner := func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{Lister: lister, Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{}}, nil
	}

	root := backup.NewRootCommand("test", newRunner)
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", t.TempDir()})

	require.NoError(t, root.Execute())

	require.NotNil(t, lister.got)
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

func TestCLIRunRejectsUnknownMetadataKind(t *testing.T) {
	root := backup.NewRootCommand("test", neverNewRunner(t))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", "config.yaml", "--dest", "/tmp/dest", "--export-metadata", "bogus"})

	err := root.Execute()

	require.ErrorIs(t, err, backup.ErrBadMetadataKind)
}

// recordingIssueExporter is a MetadataExporter for MetadataIssues that just
// records which repository paths it was asked to export.
type recordingIssueExporter struct {
	mu       *sync.Mutex
	exported *[]string
}

func newRecordingIssueExporter() recordingIssueExporter {
	return recordingIssueExporter{mu: new(sync.Mutex), exported: new([]string)}
}

func (recordingIssueExporter) Kind() backup.MetadataKind { return backup.MetadataIssues }

func (e recordingIssueExporter) Export(_ context.Context, repo backup.Repo, dir string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	*e.exported = append(*e.exported, repo.Path)
	return os.MkdirAll(dir, 0o755)
}

// TestCLIRunLeavesMetadataUnexportedByDefault guards #81's own acceptance
// criteria directly through the CLI: with no --export-metadata flag at all,
// a Runner carrying a MetadataExporter still never invokes it -- metadata
// export disabled leaves a run's behavior unchanged from before this flag
// existed.
func TestCLIRunLeavesMetadataUnexportedByDefault(t *testing.T) {
	exp := newRecordingIssueExporter()
	newRunner := func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{
			Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{},
			MetadataExporters: []backup.MetadataExporter{exp},
		}, nil
	}
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token: unused\n")

	root := backup.NewRootCommand("test", newRunner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", t.TempDir()})

	require.NoError(t, root.Execute())

	exp.mu.Lock()
	defer exp.mu.Unlock()
	require.Empty(t, *exp.exported)
	require.NotContains(t, out.String(), "metadata exported")
}

// TestCLIRunExportsMetadataWhenFlagSet is the same run with
// --export-metadata issues set, proving the flag actually reaches Options.
func TestCLIRunExportsMetadataWhenFlagSet(t *testing.T) {
	exp := newRecordingIssueExporter()
	newRunner := func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{
			Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{},
			MetadataExporters: []backup.MetadataExporter{exp},
		}, nil
	}
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token: unused\n")

	root := backup.NewRootCommand("test", newRunner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", t.TempDir(), "--export-metadata", "issues"})

	require.NoError(t, root.Execute())

	exp.mu.Lock()
	defer exp.mu.Unlock()
	require.Contains(t, *exp.exported, backup.TestActiveRepoPath)
	require.Contains(t, out.String(), "metadata exported")
}

// dirCapturingMirrorer records every dir it was asked to sync into, without
// touching disk -- these tests care only about the path, not the mirror
// contents. Run mirrors repositories concurrently, so appends are guarded by
// a mutex rather than assumed sequential.
type dirCapturingMirrorer struct {
	mu   *sync.Mutex
	dirs *[]string
}

func newDirCapturingMirrorer(dirs *[]string) dirCapturingMirrorer {
	return dirCapturingMirrorer{mu: new(sync.Mutex), dirs: dirs}
}

func (m dirCapturingMirrorer) Sync(_ context.Context, _ backup.Remote, dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	*m.dirs = append(*m.dirs, dir)
	return nil
}

func newRunnerCapturingDirs(dirs *[]string) backup.NewRunner {
	return func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{
			Lister:   newFakeLister(),
			Mirrorer: newDirCapturingMirrorer(dirs),
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

func TestCLIRunPrunesRemovedMirrorWithFlag(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_PRUNE_TOKEN\n")
	t.Setenv("TEST_PRUNE_TOKEN", "secret")
	dest := t.TempDir()
	staleDir := filepath.Join(dest, "home", backup.TestRemovedRepoPath+".git")
	require.NoError(t, os.MkdirAll(staleDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staleDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))

	newRunner := func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{}}, nil
	}

	var out bytes.Buffer
	root := backup.NewRootCommand("test", newRunner)
	root.SetOut(&out)
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", dest, "--prune-removed"})

	require.NoError(t, root.Execute())

	require.NoDirExists(t, staleDir)
	require.Contains(t, out.String(), "pruned 1")
}

func TestCLIRunLeavesRemovedMirrorAloneWithoutFlag(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_NOPRUNE_TOKEN\n")
	t.Setenv("TEST_NOPRUNE_TOKEN", "secret")
	dest := t.TempDir()
	staleDir := filepath.Join(dest, "home", backup.TestRemovedRepoPath+".git")
	require.NoError(t, os.MkdirAll(staleDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staleDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))

	newRunner := func(backup.ForgeConfig) (backup.Runner, error) {
		return backup.Runner{Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{}}, nil
	}

	root := backup.NewRootCommand("test", newRunner)
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", dest})

	require.NoError(t, root.Execute())

	require.DirExists(t, staleDir)
}

func TestCLIRunDryRunPreviewsPruneWithFlag(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_DRYPRUNE_TOKEN\n")
	t.Setenv("TEST_DRYPRUNE_TOKEN", "secret")
	dest := t.TempDir()
	staleDir := filepath.Join(dest, "home", backup.TestRemovedRepoPath+".git")
	require.NoError(t, os.MkdirAll(staleDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staleDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))

	var out bytes.Buffer
	root := backup.NewRootCommand("test", newDryRunRunner(t))
	root.SetOut(&out)
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", dest, "--dry-run", "--prune-removed"})

	require.NoError(t, root.Execute())

	require.Contains(t, out.String(), "home/"+backup.TestRemovedRepoPath+": prune")
	require.Contains(t, out.String(), "prune 1 (dry run)")
	require.DirExists(t, staleDir)
}

func TestCLIRunDryRunOmitsPruneCountWithoutFlag(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_DRYNOPRUNE_TOKEN\n")
	t.Setenv("TEST_DRYNOPRUNE_TOKEN", "secret")
	dest := t.TempDir()

	var out bytes.Buffer
	root := backup.NewRootCommand("test", newDryRunRunner(t))
	root.SetOut(&out)
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", dest, "--dry-run"})

	require.NoError(t, root.Execute())

	require.NotContains(t, out.String(), "prune")
}

// TestCLIRunDryRunPruneIgnoresStateFilter guards dryRunPrune's own copy of
// the fix in Runner.Run: a mirror for TestArchivedRepoPath is planted ahead
// of a dry run scoped to --state active, which never even lists it -- if
// dryRunPrune judged staleness against that filtered listing instead of a
// full one, it would preview deleting a repository that's still on the
// forge, only excluded from this particular run.
func TestCLIRunDryRunPruneIgnoresStateFilter(t *testing.T) {
	cfgPath := writeConfig(t, "forges:\n  - name: home\n    kind: forgejo\n    url: https://git.example.org\n    token_env: TEST_DRYPRUNESTATE_TOKEN\n")
	t.Setenv("TEST_DRYPRUNESTATE_TOKEN", "secret")
	dest := t.TempDir()
	archivedMirror := filepath.Join(dest, "home", backup.TestArchivedRepoPath+".git")
	require.NoError(t, os.MkdirAll(archivedMirror, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(archivedMirror, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))

	var out bytes.Buffer
	root := backup.NewRootCommand("test", newDryRunRunner(t))
	root.SetOut(&out)
	root.SetArgs([]string{"run", "--config", cfgPath, "--dest", dest, "--dry-run", "--state", "active", "--prune-removed"})

	require.NoError(t, root.Execute())

	require.NotContains(t, out.String(), backup.TestArchivedRepoPath+": prune")
	require.Contains(t, out.String(), "prune 0 (dry run)")
}

// perForgeLister returns a different repo per forge name, so a test can tell
// which forge's Runner produced a given mirrored path.
type perForgeLister struct {
	forge string
}

func (l perForgeLister) ListRepos(_ context.Context, _ backup.State) ([]backup.Repo, error) {
	return []backup.Repo{{Path: l.forge + "-repo"}}, nil
}

// Regression test for #32: every forge's repositories used to land under
// the first forge's destination folder instead of their own.
func TestCLIRunKeepsEachForgesDestSeparate(t *testing.T) {
	cfgPath := writeConfig(t, `
dest: /unused
forges:
  - name: a
    kind: forgejo
    url: https://git.example.org
    token_env: TEST_MULTI_FORGE_A
  - name: b
    kind: forgejo
    url: https://git.example.org
    token_env: TEST_MULTI_FORGE_B
`)
	t.Setenv("TEST_MULTI_FORGE_A", "secret")
	t.Setenv("TEST_MULTI_FORGE_B", "secret")

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

	require.Equal(t, []string{
		filepath.Join(dest, "a", "a-repo.git"),
		filepath.Join(dest, "b", "b-repo.git"),
	}, dirs)
}
