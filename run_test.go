package backup_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

// fakeMirrorer records where it was asked to sync and writes a minimal bare
// repo layout there -- just enough for the spec's FileExists checks -- so
// the spec never has to know whether it's talking to a fake or real git.
type fakeMirrorer struct{}

func (fakeMirrorer) Sync(_ context.Context, _ backup.Remote, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		return fmt.Errorf("write HEAD in %s: %w", dir, err)
	}

	return nil
}

type fakeRemoter struct{}

func (fakeRemoter) Remote(r backup.Repo) backup.Remote {
	return backup.Remote{CloneURL: "fake://" + r.Path}
}

// parentCheckingMirrorer requires dir's parent to already exist, the way a
// real `git clone --mirror` does -- it creates dir itself (the clone
// target) but never its leading directories, so a test using it fails the
// same way a race in git's own leading-directory creation would if Run
// stopped pre-creating the parent.
type parentCheckingMirrorer struct{}

func (parentCheckingMirrorer) Sync(_ context.Context, _ backup.Remote, dir string) error {
	if _, err := os.Stat(filepath.Dir(dir)); err != nil {
		return fmt.Errorf("parent of %s does not exist yet: %w", dir, err)
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		return fmt.Errorf("write HEAD in %s: %w", dir, err)
	}

	return nil
}

func TestRun(t *testing.T) {
	t.Parallel()

	backup.TestBackup(t, func(ctx context.Context, opts backup.Options) (backup.Result, error) {
		runner := backup.Runner{
			Lister:   newFakeLister(),
			Mirrorer: fakeMirrorer{},
			Remoter:  fakeRemoter{},
		}

		return runner.Run(ctx, opts)
	})
}

// blockingMirrorer waits for its context to end and reports that as the
// sync's error, the way Mirror's real git subprocess does when its context
// is canceled.
type blockingMirrorer struct{}

func (blockingMirrorer) Sync(ctx context.Context, _ backup.Remote, _ string) error {
	<-ctx.Done()

	return fmt.Errorf("sync canceled: %w", ctx.Err())
}

// nRepoLister lists n distinct, non-empty, active repositories -- enough to
// saturate a concurrency limit derived from runtime.NumCPU() on any
// realistic host.
type nRepoLister struct{ n int }

func (l nRepoLister) ListRepos(_ context.Context, _ backup.State) ([]backup.Repo, error) {
	repos := make([]backup.Repo, l.n)
	for i := range repos {
		repos[i] = backup.Repo{Path: fmt.Sprintf("repo-%d", i)}
	}

	return repos, nil
}

// barrierMirrorer reports each Sync call on arrived as it starts, then
// blocks until release is closed -- which is what lets a test prove how many
// repositories Run started concurrently, by counting how many reach the
// barrier before it's released.
type barrierMirrorer struct {
	arrived chan<- struct{}
	release <-chan struct{}
}

func (m barrierMirrorer) Sync(ctx context.Context, _ backup.Remote, _ string) error {
	m.arrived <- struct{}{}
	select {
	case <-m.release:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for release: %w", ctx.Err())
	}
}

func TestRunDefaultsConcurrencyToNumCPU(t *testing.T) {
	// serial: barrierMirrorer needs every runner-spawned goroutine to reach
	// its channel inside a 5s deadline; parallel CPU contention from other
	// tests risks flaking a check that's inherently about scheduling.
	n := runtime.GOMAXPROCS(0)
	if n < 2 {
		t.Skip("GOMAXPROCS is 1 on this host; can't distinguish this default from the old hardcoded 1")
	}

	arrived := make(chan struct{}, n)
	release := make(chan struct{})
	runner := backup.Runner{
		Lister:   nRepoLister{n: n},
		Mirrorer: barrierMirrorer{arrived: arrived, release: release},
		Remoter:  fakeRemoter{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type outcome struct {
		result backup.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := runner.Run(ctx, backup.Options{Dest: t.TempDir(), State: backup.StateAll})
		done <- outcome{result, err}
	}()

	// Concurrency defaulting to less than n (the old bug: hardcoded 1) means
	// fewer than n Sync calls ever reach the barrier at once, so this blocks
	// until the context deadline instead of every repo checking in.
	for i := range n {
		select {
		case <-arrived:
		case <-ctx.Done():
			t.Fatalf("only %d of %d repos were mirrored concurrently before the deadline", i, n)
		}
	}
	close(release)

	select {
	case o := <-done:
		require.NoError(t, o.err)
		require.Equal(t, n, o.result.Synced)
	case <-ctx.Done():
		t.Fatal("run did not finish after the concurrency barrier was released")
	}
}

func TestRunReportsProgress(t *testing.T) {
	var mu sync.Mutex
	var calls [][2]int
	progress := func(done, total int) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, [2]int{done, total})
	}

	runner := backup.Runner{
		Lister:   newFakeLister(),
		Mirrorer: fakeMirrorer{},
		Remoter:  fakeRemoter{},
	}

	_, err := runner.Run(context.Background(), backup.Options{
		Dest:     t.TempDir(),
		State:    backup.StateAll,
		Progress: progress,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, calls)

	// newFakeLister seeds 3 repos (2 non-empty, 1 empty); every call
	// reports the same total, and the last call reports every repo done,
	// including the one skipped for being empty.
	last := calls[len(calls)-1]
	require.Equal(t, 3, last[1])
	require.Equal(t, 3, last[0])
	for _, c := range calls {
		require.Equal(t, 3, c[1])
	}
}

// nestedRepoLister lists a single repository several namespace levels deep,
// none of which exist yet under any Dest a test points it at.
type nestedRepoLister struct{}

func (nestedRepoLister) ListRepos(_ context.Context, _ backup.State) ([]backup.Repo, error) {
	return []backup.Repo{{Path: "org/team/subteam/repo"}}, nil
}

// TestRunCreatesDestParentBeforeMirroring guards against #48: Run used to
// hand Mirrorer.Sync a destination whose parent directories didn't exist
// yet and rely on git's own leading-directory creation, which raced under
// concurrent clones sharing a parent and failed with "could not create
// leading directories" well into a run.
func TestRunCreatesDestParentBeforeMirroring(t *testing.T) {
	t.Parallel()

	runner := backup.Runner{
		Lister:   nestedRepoLister{},
		Mirrorer: parentCheckingMirrorer{},
		Remoter:  fakeRemoter{},
	}

	result, err := runner.Run(context.Background(), backup.Options{
		Dest:  t.TempDir(),
		State: backup.StateAll,
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.Synced)
	require.Zero(t, result.Failed)
}

// erroringLister always fails, the way a Lister does when a page fetch
// fails partway through pagination -- every adapter returns the error
// straight away rather than whatever pages it had already gathered, so
// Run never sees a truncated repo set that would look like everything else
// was deleted.
type erroringLister struct{}

var errListingFailed = errors.New("listing failed")

func (erroringLister) ListRepos(context.Context, backup.State) ([]backup.Repo, error) {
	return nil, errListingFailed
}

// TestRunDoesNotPruneWhenListingFails guards the third acceptance criterion
// of #8: a forge API failure partway through listing must leave every
// existing mirror on disk untouched, --prune-removed or not, rather than
// pruning against whatever partial set a failed listing might have produced.
func TestRunDoesNotPruneWhenListingFails(t *testing.T) {
	t.Parallel()

	dest := t.TempDir()
	stale := backup.TestSeedStaleMirror(t, dest)

	runner := backup.Runner{
		Lister:   erroringLister{},
		Mirrorer: fakeMirrorer{},
		Remoter:  fakeRemoter{},
	}

	_, err := runner.Run(context.Background(), backup.Options{
		Dest: dest, State: backup.StateAll, PruneRemoved: true,
	})

	require.ErrorIs(t, err, errListingFailed)
	require.DirExists(t, stale)
}

// TestRunCountsMirrorPrunedEvenWhenArchiveCleanupFails guards against a
// mirror that was genuinely, permanently deleted going unreported as pruned
// just because its much smaller archive residue couldn't also be cleaned up
// alongside it -- the archive path here is a non-empty directory rather
// than a file, so os.Remove on it fails with something other than "does not
// exist", the way a permissions problem on the real archive file would.
func TestRunCountsMirrorPrunedEvenWhenArchiveCleanupFails(t *testing.T) {
	t.Parallel()

	dest := t.TempDir()
	archiveDir := t.TempDir()
	staleMirror := backup.TestSeedStaleMirror(t, dest)

	staleArchiveDir := filepath.Join(archiveDir, backup.TestRemovedRepoPath+".tar.gz")
	require.NoError(t, os.MkdirAll(filepath.Join(staleArchiveDir, "not-empty"), 0o750))

	runner := backup.Runner{
		Lister:   newFakeLister(),
		Mirrorer: fakeMirrorer{},
		Remoter:  fakeRemoter{},
	}

	result, err := runner.Run(context.Background(), backup.Options{
		Dest: dest, State: backup.StateAll, ArchiveDir: archiveDir, PruneRemoved: true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Pruned)
	require.NoDirExists(t, staleMirror)
	require.DirExists(t, staleArchiveDir)
}

// recordingExporter is a MetadataExporter that records which repository
// paths it was asked to export, so a test can assert whether Run invoked it
// at all -- the thing #81's "given metadata export is not enabled, behavior
// is unchanged" acceptance criterion is actually about.
type recordingExporter struct {
	kind backup.MetadataKind

	mu       sync.Mutex
	exported []string
}

func (e *recordingExporter) Kind() backup.MetadataKind { return e.kind }

func (e *recordingExporter) Export(_ context.Context, repo backup.Repo, dir string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.exported = append(e.exported, repo.Path)

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	return nil
}

func (e *recordingExporter) exportedPaths() []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	return append([]string(nil), e.exported...)
}

func TestRunSkipsMetadataExportWhenNotRequested(t *testing.T) {
	t.Parallel()

	exp := &recordingExporter{kind: backup.MetadataIssues}
	runner := backup.Runner{
		Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{},
		MetadataExporters: []backup.MetadataExporter{exp},
	}

	result, err := runner.Run(context.Background(), backup.Options{Dest: t.TempDir(), State: backup.StateAll})
	require.NoError(t, err)

	require.Empty(t, exp.exportedPaths())
	require.Zero(t, result.MetadataExported)
}

func TestRunExportsRequestedMetadataKind(t *testing.T) {
	t.Parallel()

	exp := &recordingExporter{kind: backup.MetadataIssues}
	runner := backup.Runner{
		Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{},
		MetadataExporters: []backup.MetadataExporter{exp},
	}

	dest := t.TempDir()
	result, err := runner.Run(context.Background(), backup.Options{
		Dest: dest, State: backup.StateActive, ExportMetadata: []backup.MetadataKind{backup.MetadataIssues},
	})
	require.NoError(t, err)

	require.Contains(t, exp.exportedPaths(), backup.TestActiveRepoPath)
	require.Equal(t, 1, result.MetadataExported)
	require.DirExists(t, filepath.Join(dest, backup.TestActiveRepoPath+".metadata", "issues"))
}

// failingExporter always fails, the way a real forge's Export does when the
// API call behind it errors.
type failingExporter struct{ kind backup.MetadataKind }

func (e failingExporter) Kind() backup.MetadataKind { return e.kind }

func (failingExporter) Export(context.Context, backup.Repo, string) error {
	return errExportFailed
}

var errExportFailed = errors.New("export failed")

func TestRunCountsMirrorFailedWhenMetadataExportFails(t *testing.T) {
	t.Parallel()

	runner := backup.Runner{
		Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{},
		MetadataExporters: []backup.MetadataExporter{failingExporter{kind: backup.MetadataIssues}},
	}

	result, err := runner.Run(context.Background(), backup.Options{
		Dest: t.TempDir(), State: backup.StateActive, ExportMetadata: []backup.MetadataKind{backup.MetadataIssues},
	})
	require.NoError(t, err)

	require.Equal(t, 1, result.Failed)
	require.Zero(t, result.MetadataExported)
}

// TestRunCountsMirrorFailedWhenMetadataDirCannotBeCreated guards the case
// where exportMetadata's own os.MkdirAll fails -- here because a plain file
// already sits where the metadata directory needs to go, the way a
// permissions problem or an existing non-directory file at that path would
// in the real world.
func TestRunCountsMirrorFailedWhenMetadataDirCannotBeCreated(t *testing.T) {
	t.Parallel()

	dest := t.TempDir()
	blocked := filepath.Join(dest, backup.TestActiveRepoPath+".metadata")
	require.NoError(t, os.MkdirAll(filepath.Dir(blocked), 0o750))
	require.NoError(t, os.WriteFile(blocked, []byte("not a directory"), 0o600))

	exp := &recordingExporter{kind: backup.MetadataIssues}
	runner := backup.Runner{
		Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{},
		MetadataExporters: []backup.MetadataExporter{exp},
	}

	result, err := runner.Run(context.Background(), backup.Options{
		Dest: dest, State: backup.StateActive, ExportMetadata: []backup.MetadataKind{backup.MetadataIssues},
	})
	require.NoError(t, err)

	require.Equal(t, 1, result.Failed)
	require.Empty(t, exp.exportedPaths(), "Export must never be called once its directory couldn't be created")
}

func TestRunSkipsMetadataExportForAnExporterOfAKindNotRequested(t *testing.T) {
	t.Parallel()

	// A Runner can carry an exporter for a kind Options.ExportMetadata
	// didn't ask for -- #82/#83/#84 will each add their own kind to the
	// same Runner -- and only the requested one should ever run.
	requested := &recordingExporter{kind: backup.MetadataIssues}
	other := &recordingExporter{kind: backup.MetadataKind("pull-requests")}
	runner := backup.Runner{
		Lister: newFakeLister(), Mirrorer: fakeMirrorer{}, Remoter: fakeRemoter{},
		MetadataExporters: []backup.MetadataExporter{requested, other},
	}

	_, err := runner.Run(context.Background(), backup.Options{
		Dest: t.TempDir(), State: backup.StateActive, ExportMetadata: []backup.MetadataKind{backup.MetadataIssues},
	})
	require.NoError(t, err)

	require.NotEmpty(t, requested.exportedPaths())
	require.Empty(t, other.exportedPaths())
}

func TestRunTimeout(t *testing.T) {
	// serial: blockingMirrorer races a real 1ms context timeout against
	// scheduling; parallel CPU contention risks flaking that margin.
	runner := backup.Runner{
		Lister:   newFakeLister(),
		Mirrorer: blockingMirrorer{},
		Remoter:  fakeRemoter{},
	}

	result, err := runner.Run(context.Background(), backup.Options{
		Dest:    t.TempDir(),
		State:   backup.StateActive,
		Timeout: time.Millisecond,
	})
	require.NoError(t, err)

	require.Equal(t, 1, result.Failed)
}
