package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The paths every Lister's and Runner's fixture data is expected to seed.
// An adapter's own fixture-seeding driver (the fake, Forgejo, GitLab) is
// responsible for creating repositories under these exact paths, which is
// what lets the suites below run unchanged against any of them.
const (
	TestActiveRepoPath   = "team/active-repo"
	TestArchivedRepoPath = "team/archived-repo"
	TestEmptyRepoPath    = "team/empty-repo"
)

// TestLister runs the behaviour every Lister must satisfy, whether it's
// backed by a fake or a real forge. Exported here, rather than living in a
// _test.go file, so an adapter under internal/ can run the same suite
// against itself -- otherwise an unexported helper in this package's own
// tests would be unreachable from anywhere that isn't this package.
func TestLister(t *testing.T, newLister func(t *testing.T) Lister) {
	t.Helper()

	t.Run("lists active repos", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), StateActive)
		require.NoError(t, err)

		paths := repoPaths(repos)
		require.Contains(t, paths, TestActiveRepoPath)
		require.NotContains(t, paths, TestArchivedRepoPath)
	})

	t.Run("lists archived repos", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), StateArchived)
		require.NoError(t, err)

		paths := repoPaths(repos)
		require.Contains(t, paths, TestArchivedRepoPath)
		require.NotContains(t, paths, TestActiveRepoPath)
	})

	t.Run("lists all repos", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), StateAll)
		require.NoError(t, err)

		paths := repoPaths(repos)
		require.Contains(t, paths, TestActiveRepoPath)
		require.Contains(t, paths, TestArchivedRepoPath)
	})

	t.Run("carries the full namespace path", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), StateAll)
		require.NoError(t, err)

		require.Equal(t, TestActiveRepoPath, findRepo(t, repos, TestActiveRepoPath).Path)
	})

	t.Run("reports empty repos", func(t *testing.T) {
		l := newLister(t)
		repos, err := l.ListRepos(context.Background(), StateAll)
		require.NoError(t, err)

		require.True(t, findRepo(t, repos, TestEmptyRepoPath).Empty)
	})
}

func repoPaths(repos []Repo) []string {
	paths := make([]string, len(repos))
	for i, r := range repos {
		paths[i] = r.Path
	}

	return paths
}

func findRepo(t *testing.T, repos []Repo, path string) Repo {
	t.Helper()
	for _, r := range repos {
		if r.Path == path {
			return r
		}
	}
	t.Fatalf("no repo with path %q in %v", path, repoPaths(repos))

	return Repo{}
}

// TestDriver runs a backup the way Runner.Run does, whether that's against
// an in-memory fake or a real forge container.
type TestDriver func(ctx context.Context, opts Options) (Result, error)

// TestBackup runs the specification every forge's backup pipeline must
// satisfy, in domain terms: it talks only about a destination directory and
// the state it asks for, so the same spec runs unchanged against a fake or
// a real forge.
func TestBackup(t *testing.T, run TestDriver) {
	t.Helper()
	t.Run("backs up active repos", func(t *testing.T) { backsUpActiveRepos(t, run) })
	t.Run("keeps the forge folder structure", func(t *testing.T) { keepsTheForgeFolderStructure(t, run) })
	t.Run("leaves archived repos alone when active only", func(t *testing.T) { leavesArchivedReposAloneWhenActiveOnly(t, run) })
	t.Run("skips empty repos", func(t *testing.T) { skipsEmptyRepos(t, run) })
	t.Run("archives only the selected repos", func(t *testing.T) { archivesOnlyTheSelectedRepos(t, run) })
	t.Run("does not persist a mirror for an archived repo that gets archived", func(t *testing.T) {
		doesNotPersistMirrorForArchivedRepo(t, run)
	})
	t.Run("leaves a removed repository's mirror alone without --prune-removed", func(t *testing.T) {
		leavesRemovedMirrorAloneWithoutFlag(t, run)
	})
	t.Run("warns about a removed repository's mirror without --prune-removed", func(t *testing.T) {
		warnsAboutRemovedMirrorWithoutFlag(t, run)
	})
	t.Run("prunes a removed repository's mirror and archive with --prune-removed", func(t *testing.T) {
		prunesRemovedMirrorWithFlag(t, run)
	})
	t.Run("does not prune a mirror merely excluded by --state", func(t *testing.T) {
		doesNotPruneMirrorExcludedByState(t, run)
	})
}

// TestRemovedRepoPath is a repository path no fixture set seeds -- every
// suite's Lister reports the fixed TestActiveRepoPath/TestArchivedRepoPath/
// TestEmptyRepoPath set and nothing under "team/removed-repo", so a mirror
// or archive planted at this path ahead of a run is exactly what a
// repository deleted or renamed upstream since the last run looks like.
const TestRemovedRepoPath = "team/removed-repo"

// TestSeedStaleMirror creates a mirror directory at dest for
// TestRemovedRepoPath, the way a previous run would have left one for a
// repository that has since been deleted or renamed upstream. Exported for
// the same reason as TestLister/TestBackup: a test outside this package (an
// adapter under internal/, or this package's own external test package)
// needs it too.
func TestSeedStaleMirror(t *testing.T, dest string) string {
	t.Helper()
	dir := filepath.Join(dest, TestRemovedRepoPath+".git")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600))

	return dir
}

// TestSeedStaleArchive creates an archive tarball at archiveDir for
// TestRemovedRepoPath, the way --archive would have left one for a
// repository that has since been deleted or renamed upstream.
func TestSeedStaleArchive(t *testing.T, archiveDir string) string {
	t.Helper()
	file := filepath.Join(archiveDir, TestRemovedRepoPath+".tar.gz")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o750))
	require.NoError(t, os.WriteFile(file, []byte("not a real tarball"), 0o600))

	return file
}

func leavesRemovedMirrorAloneWithoutFlag(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()
	stale := TestSeedStaleMirror(t, dest)

	_, err := run(context.Background(), Options{Dest: dest, State: StateAll})
	require.NoError(t, err)

	require.DirExists(t, stale)
}

func warnsAboutRemovedMirrorWithoutFlag(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()
	TestSeedStaleMirror(t, dest)

	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))

	_, err := run(context.Background(), Options{Dest: dest, State: StateAll, Log: log})
	require.NoError(t, err)

	require.Contains(t, logs.String(), TestRemovedRepoPath)
	require.Contains(t, logs.String(), "level=WARN")
}

func prunesRemovedMirrorWithFlag(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()
	archiveDir := t.TempDir()
	staleMirror := TestSeedStaleMirror(t, dest)
	staleArchive := TestSeedStaleArchive(t, archiveDir)

	result, err := run(context.Background(), Options{
		Dest: dest, State: StateAll, ArchiveDir: archiveDir, PruneRemoved: true,
	})
	require.NoError(t, err)

	require.Equal(t, 1, result.Pruned)
	require.NoDirExists(t, staleMirror)
	require.NoFileExists(t, staleArchive)

	// A repository the Lister still reports is left alone.
	require.FileExists(t, filepath.Join(dest, TestActiveRepoPath+".git", "HEAD"))
}

// doesNotPruneMirrorExcludedByState plants a mirror for TestArchivedRepoPath
// -- a repository the fixture set genuinely still reports, just not under
// StateActive -- and runs with State: StateActive and PruneRemoved: true.
// Staleness has to be judged against a full listing regardless of State, or
// this would read as "gone" and prune it, when it was only excluded from
// this run's own mirroring pass.
func doesNotPruneMirrorExcludedByState(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()
	archivedMirror := filepath.Join(dest, TestArchivedRepoPath+".git")
	require.NoError(t, os.MkdirAll(archivedMirror, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(archivedMirror, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600))

	_, err := run(context.Background(), Options{Dest: dest, State: StateActive, PruneRemoved: true})
	require.NoError(t, err)

	require.DirExists(t, archivedMirror)
}

func backsUpActiveRepos(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()

	_, err := run(context.Background(), Options{Dest: dest, State: StateActive})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dest, TestActiveRepoPath+".git", "HEAD"))
}

func keepsTheForgeFolderStructure(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()

	_, err := run(context.Background(), Options{Dest: dest, State: StateAll})
	require.NoError(t, err)

	require.DirExists(t, filepath.Join(dest, "team"))
	require.FileExists(t, filepath.Join(dest, TestActiveRepoPath+".git", "HEAD"))
}

func leavesArchivedReposAloneWhenActiveOnly(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()

	_, err := run(context.Background(), Options{Dest: dest, State: StateActive})
	require.NoError(t, err)

	require.NoFileExists(t, filepath.Join(dest, TestArchivedRepoPath+".git", "HEAD"))
}

func archivesOnlyTheSelectedRepos(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()
	archiveDir := t.TempDir()

	result, err := run(context.Background(), Options{
		Dest:       dest,
		State:      StateAll,
		Archive:    ArchiveArchived,
		ArchiveDir: archiveDir,
	})
	require.NoError(t, err)

	require.Equal(t, 1, result.Archived)
	require.FileExists(t, filepath.Join(archiveDir, TestArchivedRepoPath+".tar.gz"))
	require.NoFileExists(t, filepath.Join(archiveDir, TestActiveRepoPath+".tar.gz"))
}

func doesNotPersistMirrorForArchivedRepo(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()
	archiveDir := t.TempDir()

	_, err := run(context.Background(), Options{
		Dest:       dest,
		State:      StateAll,
		Archive:    ArchiveAll,
		ArchiveDir: archiveDir,
	})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(archiveDir, TestArchivedRepoPath+".tar.gz"))
	require.NoDirExists(t, filepath.Join(dest, TestArchivedRepoPath+".git"))

	// An active repo still gets both -- it's still being written to, so it
	// still needs an incrementally updated mirror, not just a snapshot.
	require.FileExists(t, filepath.Join(dest, TestActiveRepoPath+".git", "HEAD"))
	require.FileExists(t, filepath.Join(archiveDir, TestActiveRepoPath+".tar.gz"))
}

func skipsEmptyRepos(t *testing.T, run TestDriver) {
	t.Helper()
	dest := t.TempDir()

	result, err := run(context.Background(), Options{Dest: dest, State: StateAll})
	require.NoError(t, err)

	require.NoDirExists(t, filepath.Join(dest, TestEmptyRepoPath+".git"))
	require.GreaterOrEqual(t, result.Skipped, 1)
}

// The issues every MetadataIssues exporter's own fixture set is expected to
// seed on TestActiveRepoPath, on top of what TestLister already requires:
// an open issue carrying one comment, and a closed issue carrying none, so
// TestIssueExporter can prove both a populated comment list and #81's "an
// issue with no comments is still written" requirement from the one
// fixture set.
const (
	TestIssueOpenTitle   = "open issue"
	TestIssueClosedTitle = "closed issue"
	TestIssueCommentBody = "a comment on the open issue"
)

// TestIssueExporter runs the specification every MetadataIssues
// MetadataExporter must satisfy, whether it's backed by a fake or a real
// forge: newExporter builds the exporter under test, seeded (per the
// TestIssueOpenTitle/TestIssueClosedTitle doc comment above) against
// TestActiveRepoPath the same way every Lister fixture already is.
func TestIssueExporter(t *testing.T, newExporter func(t *testing.T) MetadataExporter) {
	t.Helper()

	t.Run("reports its kind", func(t *testing.T) {
		exp := newExporter(t)
		require.Equal(t, MetadataIssues, exp.Kind())
	})

	t.Run("writes every issue, including one with no comments, as its own documented file", func(t *testing.T) {
		exp := newExporter(t)
		dir := t.TempDir()

		err := exp.Export(context.Background(), Repo{Path: TestActiveRepoPath}, dir)
		require.NoError(t, err)

		issues := readIssues(t, dir)

		open := findIssueByTitle(t, issues, TestIssueOpenTitle)
		require.Equal(t, "open", open.State)
		require.NotEmpty(t, open.Author)
		require.False(t, open.CreatedAt.IsZero())
		require.False(t, open.UpdatedAt.IsZero())
		require.Nil(t, open.ClosedAt)
		require.Len(t, open.Comments, 1)
		require.Equal(t, TestIssueCommentBody, open.Comments[0].Body)
		require.NotEmpty(t, open.Comments[0].Author)
		require.False(t, open.Comments[0].CreatedAt.IsZero())

		closed := findIssueByTitle(t, issues, TestIssueClosedTitle)
		require.Equal(t, "closed", closed.State)
		require.NotNil(t, closed.ClosedAt)
		require.NotNil(t, closed.Comments, "an issue with no comments must still be written with an empty list, not skipped")
		require.Empty(t, closed.Comments)
	})
}

// readIssues reads every "*.json" file WriteIssue wrote into dir back into
// an Issue, in no particular order.
func readIssues(t *testing.T, dir string) []Issue {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	issues := make([]Issue, 0, len(entries))
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // e.Name() came straight from os.ReadDir(dir), not untrusted input
		require.NoError(t, err)

		var issue Issue
		require.NoError(t, json.Unmarshal(data, &issue))
		issues = append(issues, issue)
	}

	return issues
}

func findIssueByTitle(t *testing.T, issues []Issue, title string) Issue {
	t.Helper()
	for _, i := range issues {
		if i.Title == title {
			return i
		}
	}
	t.Fatalf("no issue titled %q among %d issues", title, len(issues))

	return Issue{}
}
