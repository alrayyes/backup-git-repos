package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

// Mirrorer keeps a repository's bare mirror up to date on disk.
type Mirrorer interface {
	Sync(ctx context.Context, r Remote, dir string) error
}

// Remoter builds the clone Remote for a repository.
type Remoter interface {
	Remote(repo Repo) Remote
}

// ArchiveSelection selects which repositories also get written out as a
// tar.gz alongside their mirror, in addition to being mirrored.
type ArchiveSelection int

// The values ArchiveSelection can hold.
const (
	ArchiveNone ArchiveSelection = iota
	ArchiveAll
	ArchiveActive
	ArchiveArchived
)

func (a ArchiveSelection) wants(archived bool) bool {
	switch a {
	case ArchiveAll:
		return true
	case ArchiveActive:
		return !archived
	case ArchiveArchived:
		return archived
	default:
		return false
	}
}

// Options configures a Run.
type Options struct {
	Dest        string
	State       State
	Archive     ArchiveSelection
	ArchiveDir  string
	Concurrency int
	Timeout     time.Duration
	Log         *slog.Logger

	// PruneRemoved removes a mirror (and its archive, if one exists) once
	// its repository no longer appears on the forge at all. Off by
	// default: a backup tool deleting things on its own is one bad API
	// response away from deleting everything, so this is opt-in, and
	// staleMirrors only ever acts on a listing Run is confident is
	// complete -- see Run's own doc comment. Staleness is always judged
	// against a full listing regardless of State, never against State's
	// own filtered subset, so running with PruneRemoved alongside
	// State other than StateAll still only removes a mirror for a
	// repository actually gone from the forge, not one this run's own
	// State filter merely excluded.
	PruneRemoved bool

	// ExportMetadata selects which kinds of forge metadata -- issues today,
	// pull/merge requests, releases and CI/CD config once #82/#83/#84 add
	// their own MetadataExporter -- get exported alongside each mirrored
	// repository's own bare mirror. Empty (the zero value) is metadata
	// export disabled: a repository skipped for being empty is never
	// mirrored either (see Run's own doc comment), so it never reaches
	// mirrorRepo's exportMetadata step regardless of this field.
	ExportMetadata []MetadataKind

	// Progress, if set, is called every time a repository is skipped or
	// finishes mirroring (successfully or not), reporting how many of the
	// total have been accounted for so far. Run serializes calls to it, so
	// it doesn't need its own locking even though repositories are
	// mirrored concurrently.
	Progress func(done, total int)
}

// Result reports what a Run did.
type Result struct {
	Synced           int
	Skipped          int
	Failed           int
	Archived         int
	Pruned           int
	MetadataExported int
}

// Runner backs up one forge: it lists repositories, then mirrors each one
// into a destination tree that mirrors the forge's own namespace structure.
type Runner struct {
	Lister   Lister
	Mirrorer Mirrorer
	Remoter  Remoter

	// MetadataExporters are the forge's own exporters -- one per
	// MetadataKind it supports, issues today -- run against every
	// non-empty repository whose kind Options.ExportMetadata selects.
	// Wiring one in here doesn't itself turn its export on: that's
	// Options.ExportMetadata's job, so the composition root can wire every
	// adapter's exporters in unconditionally and let the flag decide.
	MetadataExporters []MetadataExporter
}

// Run lists repositories filtered by opts.State and mirrors each one,
// skipping repositories with no refs -- a mirror of one would just confuse
// the next refresh. It stays synchronous even though it mirrors up to
// opts.Concurrency repositories at once: the call blocks until every repo
// has been attempted, and the caller decides whether to run it concurrently
// with anything else. One repository failing is logged and doesn't stop the
// rest; Result.Failed is how the caller finds out afterwards.
//
// A mirror or archive already on disk for a repository the listing no
// longer reports is either pruned (opts.PruneRemoved) or, by default, left
// alone with a warning logged naming it. Staleness is judged against a full
// (StateAll) listing regardless of opts.State, fetched with a second call
// when opts.State itself asked for a filtered subset -- otherwise a run
// scoped to, say, --state active would read every archived repository as
// "gone" and prune it, when it was merely excluded from this run's own
// mirroring pass. Either way this only ever considers a listing Run already
// has in hand complete: r.Lister.ListRepos itself returning an error --
// including one from a page fetch failing partway through pagination --
// aborts Run before any of this runs, the same as it always has, so a
// partial listing never gets mistaken for "everything else was deleted".
func (r Runner) Run(ctx context.Context, opts Options) (Result, error) {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}

	repos, canonical, err := r.listRepos(ctx, opts.State)
	if err != nil {
		return Result{}, err
	}

	stale, err := staleMirrors(opts.Dest, opts.ArchiveDir, canonical)
	if err != nil {
		return Result{}, fmt.Errorf("run: %w", err)
	}

	concurrency := opts.Concurrency
	if concurrency < 1 {
		concurrency = runtime.GOMAXPROCS(0)
	}

	counters := &runCounters{total: len(repos), progress: opts.Progress}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for _, repo := range repos {
		if repo.Empty {
			log.Warn("skipping empty repository", "path", repo.Path)
			counters.skipped.Add(1)
			counters.reportDone()

			continue
		}

		g.Go(func() error {
			defer counters.reportDone()
			r.runRepo(ctx, opts, repo, log, counters)

			return nil
		})
	}

	_ = g.Wait()

	pruned := prune(opts, stale, log)

	return Result{
		Synced:           int(counters.synced.Load()),
		Skipped:          int(counters.skipped.Load()),
		Failed:           int(counters.failed.Load()),
		Archived:         int(counters.archived.Load()),
		Pruned:           pruned,
		MetadataExported: int(counters.metadataExported.Load()),
	}, nil
}

// listRepos returns opts.State's own filtered listing alongside a full
// (StateAll) canonical listing staleMirrors judges staleness against --
// the same listing when state is already StateAll, a second call
// otherwise, since a run scoped to a filtered state must still see every
// repository on the forge to tell a merely-excluded one apart from one
// that's actually gone.
func (r Runner) listRepos(ctx context.Context, state State) (repos, canonical []Repo, err error) {
	repos, err = r.Lister.ListRepos(ctx, state)
	if err != nil {
		return nil, nil, fmt.Errorf("run: %w", err)
	}

	canonical = repos
	if state != StateAll {
		canonical, err = r.Lister.ListRepos(ctx, StateAll)
		if err != nil {
			return nil, nil, fmt.Errorf("run: %w", err)
		}
	}

	return repos, canonical, nil
}

// runCounters tallies what Run's concurrent workers did, and reports
// progress as each one finishes. Every field is safe for concurrent use,
// so a worker goroutine never needs its own locking beyond what's already
// here -- progressMu serializes both incrementing done and the call into
// opts.Progress, which Run's own doc comment promises are serialized for
// the caller. Incrementing and reporting have to share one critical
// section: done is still an atomic.Int64 so its other readers (the final
// summary) need no lock of their own, but if reportDone incremented it
// outside progressMu, two goroutines could reach the lock in an order
// different from the one they incremented in, and the caller could see a
// done that goes backwards or a final call that doesn't report done ==
// total.
type runCounters struct {
	synced, skipped, failed, archived, metadataExported, done atomic.Int64
	total                                                     int
	progressMu                                                sync.Mutex
	progress                                                  func(done, total int)
}

func (c *runCounters) reportDone() {
	c.progressMu.Lock()
	defer c.progressMu.Unlock()

	n := int(c.done.Add(1))
	if c.progress != nil {
		c.progress(n, c.total)
	}
}

// runRepo mirrors one repository and folds its outcome into c. It's the
// per-repository unit of work Run's errgroup runs concurrently, bounded
// by opts.Concurrency.
func (r Runner) runRepo(ctx context.Context, opts Options, repo Repo, log *slog.Logger, c *runCounters) {
	repoCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		repoCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	outcome := r.mirrorRepo(repoCtx, opts, repo, log)
	if outcome.synced {
		c.synced.Add(1)
	}
	if outcome.archived {
		c.archived.Add(1)
	}
	if outcome.failed {
		c.failed.Add(1)
	}
	c.metadataExported.Add(int64(outcome.metadataExported))
}

// mirrorOutcome reports what mirrorRepo actually did, so Run's caller can
// update its own counters without repeating mirrorRepo's stage-by-stage
// logic: a repository can be synced and still count as failed, when
// archiving or exporting metadata for it afterwards is what went wrong.
type mirrorOutcome struct {
	synced           bool
	archived         bool
	failed           bool
	metadataExported int
}

// mirrorRepo mirrors one repository, and archives it too if wantArchive
// selects it. It never returns an error -- every failure is logged and
// reported through the returned mirrorOutcome instead, which is what lets
// Run keep going after one repository fails rather than aborting the rest.
func (r Runner) mirrorRepo(ctx context.Context, opts Options, repo Repo, log *slog.Logger) mirrorOutcome {
	wantArchive := opts.Archive.wants(repo.Archived)

	// An archived repository selected for archiving is done changing, so
	// there's no point keeping an incrementally updated mirror alongside
	// its tar.gz too -- mirror into a scratch dir instead of the
	// destination tree, and let it go with the scratch dir once the
	// archive is written.
	dir := mirrorPath(opts.Dest, repo.Path)
	if repo.Archived && wantArchive {
		scratch, err := os.MkdirTemp("", "backup-git-repos-*")
		if err != nil {
			log.Error("mirror failed", "path", repo.Path, "error", err)

			return mirrorOutcome{failed: true}
		}
		defer func() { _ = os.RemoveAll(scratch) }()
		dir = filepath.Join(scratch, filepath.Base(dir))
	}

	if err := os.MkdirAll(filepath.Dir(dir), 0o750); err != nil {
		log.Error("mirror failed", "path", repo.Path, "error", err)

		return mirrorOutcome{failed: true}
	}

	log.Debug("mirroring", "path", repo.Path)
	if err := r.Mirrorer.Sync(ctx, r.Remoter.Remote(repo), dir); err != nil {
		log.Error("mirror failed", "path", repo.Path, "error", err)

		return mirrorOutcome{failed: true}
	}

	outcome := mirrorOutcome{synced: true}
	outcome.metadataExported, outcome.failed = r.exportMetadata(ctx, opts, repo, log)

	if !wantArchive {
		return outcome
	}

	archiveOut := archivePath(opts.ArchiveDir, repo.Path)
	log.Debug("archiving", "path", repo.Path)
	if err := archiveRepo(dir, archiveOut); err != nil {
		log.Error("archive failed", "path", repo.Path, "error", err)
		outcome.failed = true

		return outcome
	}

	outcome.archived = true

	return outcome
}

// exportMetadata runs every one of r.MetadataExporters whose Kind
// opts.ExportMetadata selected, each into its own sibling directory under
// metadataDir(opts.Dest, repo.Path) -- see MetadataExporter's own doc
// comment for the full on-disk convention. It always writes under
// opts.Dest, never a scratch directory, even for an archived repository
// mirrored into one (see mirrorRepo above): exported metadata isn't part of
// the git mirror, so it isn't something --archive's "no persisted mirror"
// choice applies to. One exporter failing is logged and doesn't stop the
// rest, the same "one repository's problem doesn't stop the run" shape
// mirrorRepo already gives archiving.
func (r Runner) exportMetadata(ctx context.Context, opts Options, repo Repo, log *slog.Logger) (exported int, failed bool) {
	for _, exp := range r.MetadataExporters {
		if !wantsMetadata(opts.ExportMetadata, exp.Kind()) {
			continue
		}

		dir := filepath.Join(metadataDir(opts.Dest, repo.Path), string(exp.Kind()))
		if err := os.MkdirAll(dir, 0o750); err != nil {
			log.Error("metadata export failed", "path", repo.Path, "kind", exp.Kind(), "error", err)
			failed = true

			continue
		}

		log.Debug("exporting metadata", "path", repo.Path, "kind", exp.Kind())
		if err := exp.Export(ctx, repo, dir); err != nil {
			log.Error("metadata export failed", "path", repo.Path, "kind", exp.Kind(), "error", err)
			failed = true

			continue
		}
		exported++
	}

	return exported, failed
}

// prune acts on the stale paths staleMirrors already found, either removing
// each one (opts.PruneRemoved) or just warning about it -- deliberately
// never both silently doing neither and never erroring the run over a
// single failed removal, since one repository's mirror being hard to
// delete (a permissions problem, say) shouldn't stop the rest.
func prune(opts Options, stale []string, log *slog.Logger) int {
	var pruned int
	for _, path := range stale {
		if !opts.PruneRemoved {
			log.Warn("mirror's repository no longer found on forge", "path", path)

			continue
		}

		// The mirror directory is the primary, and much larger, thing
		// being pruned -- once it's gone the repository is genuinely
		// pruned even if its much smaller archive residue can't be
		// cleaned up alongside it, so a failure there doesn't take back
		// the count or leave the mirror behind out of caution.
		if err := os.RemoveAll(mirrorPath(opts.Dest, path)); err != nil {
			log.Error("prune failed", "path", path, "error", err)

			continue
		}
		pruned++
		log.Info("pruned removed repository", "path", path)

		archiveFile := archivePath(opts.ArchiveDir, path)
		if err := os.Remove(archiveFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Error("pruned mirror but its archive could not be removed", "path", path, "error", err)
		}
	}

	return pruned
}

// mirrorPath and archivePath are the two file layouts a repository path can
// land at on disk -- shared by mirrorRepo, prune and staleMirrors so the
// mirror/archive naming convention only ever needs changing in one place.
func mirrorPath(dest, path string) string {
	return filepath.Join(dest, filepath.FromSlash(path)+".git")
}

func archivePath(archiveDir, path string) string {
	return filepath.Join(archiveDir, filepath.FromSlash(path)+".tar.gz")
}

// staleMirrors returns, in sorted order, the repository paths for which a
// mirror or an archive tarball sits on disk under dest or archiveDir but
// which repos -- a full listing of the forge -- no longer reports: a
// repository deleted or renamed upstream since the last run left its mirror
// behind with nothing left to refresh it. Neither directory existing yet
// (a first run) is not an error -- there's nothing to compare against. The
// two directories are unrelated trees, so they're scanned concurrently.
func staleMirrors(dest, archiveDir string, repos []Repo) ([]string, error) {
	wanted := make(map[string]bool, len(repos))
	for _, r := range repos {
		wanted[r.Path] = true
	}

	var mirrors, archives []string
	g := new(errgroup.Group)
	g.Go(func() error {
		var err error
		mirrors, err = onDiskPaths(dest, ".git", isMirrorDir)
		if err != nil {
			return fmt.Errorf("scan %s: %w", dest, err)
		}

		return nil
	})
	g.Go(func() error {
		var err error
		archives, err = onDiskPaths(archiveDir, ".tar.gz", isArchiveFile)
		if err != nil {
			return fmt.Errorf("scan %s: %w", archiveDir, err)
		}

		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("scan for stale mirrors: %w", err)
	}

	seen := make(map[string]bool, len(mirrors)+len(archives))
	var stale []string
	for _, path := range slices.Concat(mirrors, archives) {
		if wanted[path] || seen[path] {
			continue
		}
		seen[path] = true
		stale = append(stale, path)
	}
	slices.Sort(stale)

	return stale, nil
}

func isMirrorDir(d fs.DirEntry) bool { return d.IsDir() }

func isArchiveFile(d fs.DirEntry) bool { return !d.IsDir() }

// onDiskPaths walks root for entries matching keep whose name ends in
// suffix, and returns the repository path each one represents -- root
// itself made relative, with suffix trimmed and separators normalised to
// "/" so it matches a Repo.Path from any Lister. A matching directory is
// never descended into: a mirror's own ".git" internals aren't repository
// namespace, and a bare mirror never nests another one inside it.
func onDiskPaths(root, suffix string, keep func(fs.DirEntry) bool) ([]string, error) {
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("stat %s: %w", root, err)
	}

	var paths []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root || !strings.HasSuffix(d.Name(), suffix) || !keep(d) {
			return nil
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return fmt.Errorf("relativize %s: %w", p, err)
		}
		paths = append(paths, filepath.ToSlash(strings.TrimSuffix(rel, suffix)))
		if d.IsDir() {
			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}

	return paths, nil
}

// archiveRepo creates the archive's parent directory before writing to it --
// the destination tree mirrors the forge's namespace, so a repo two levels
// deep needs the same nesting created in the archive directory too.
func archiveRepo(dir, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(out), err)
	}

	return Archive(dir, out)
}
