package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
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

	// Progress, if set, is called every time a repository is skipped or
	// finishes mirroring (successfully or not), reporting how many of the
	// total have been accounted for so far. Run serializes calls to it, so
	// it doesn't need its own locking even though repositories are
	// mirrored concurrently.
	Progress func(done, total int)
}

// Result reports what a Run did.
type Result struct {
	Synced   int
	Skipped  int
	Failed   int
	Archived int
}

// Runner backs up one forge: it lists repositories, then mirrors each one
// into a destination tree that mirrors the forge's own namespace structure.
type Runner struct {
	Lister   Lister
	Mirrorer Mirrorer
	Remoter  Remoter
}

// Run lists repositories filtered by opts.State and mirrors each one,
// skipping repositories with no refs -- a mirror of one would just confuse
// the next refresh. It stays synchronous even though it mirrors up to
// opts.Concurrency repositories at once: the call blocks until every repo
// has been attempted, and the caller decides whether to run it concurrently
// with anything else. One repository failing is logged and doesn't stop the
// rest; Result.Failed is how the caller finds out afterwards.
func (r Runner) Run(ctx context.Context, opts Options) (Result, error) {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}

	repos, err := r.Lister.ListRepos(ctx, opts.State)
	if err != nil {
		return Result{}, fmt.Errorf("run: %w", err)
	}

	concurrency := opts.Concurrency
	if concurrency < 1 {
		concurrency = runtime.GOMAXPROCS(0)
	}

	var synced, skipped, failed, archived, done atomic.Int64
	total := len(repos)
	var progressMu sync.Mutex
	reportDone := func() {
		n := int(done.Add(1))
		if opts.Progress == nil {
			return
		}
		progressMu.Lock()
		defer progressMu.Unlock()
		opts.Progress(n, total)
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for _, repo := range repos {
		if repo.Empty {
			log.Warn("skipping empty repository", "path", repo.Path)
			skipped.Add(1)
			reportDone()
			continue
		}

		g.Go(func() error {
			defer reportDone()

			repoCtx := ctx
			if opts.Timeout > 0 {
				var cancel context.CancelFunc
				repoCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
				defer cancel()
			}

			wantArchive := opts.Archive.wants(repo.Archived)

			// An archived repository selected for archiving is done
			// changing, so there's no point keeping an incrementally
			// updated mirror alongside its tar.gz too -- mirror into a
			// scratch dir instead of the destination tree, and let it go
			// with the scratch dir once the archive is written.
			dir := filepath.Join(opts.Dest, filepath.FromSlash(repo.Path)+".git")
			if repo.Archived && wantArchive {
				scratch, err := os.MkdirTemp("", "backup-git-repos-*")
				if err != nil {
					log.Error("mirror failed", "path", repo.Path, "error", err)
					failed.Add(1)
					return nil
				}
				defer func() { _ = os.RemoveAll(scratch) }()
				dir = filepath.Join(scratch, filepath.Base(dir))
			}

			log.Debug("mirroring", "path", repo.Path)
			if err := r.Mirrorer.Sync(repoCtx, r.Remoter.Remote(repo), dir); err != nil {
				log.Error("mirror failed", "path", repo.Path, "error", err)
				failed.Add(1)
				return nil
			}
			synced.Add(1)

			if wantArchive {
				archiveOut := filepath.Join(opts.ArchiveDir, filepath.FromSlash(repo.Path)+".tar.gz")
				log.Debug("archiving", "path", repo.Path)
				if err := archiveRepo(dir, archiveOut); err != nil {
					log.Error("archive failed", "path", repo.Path, "error", err)
					failed.Add(1)
					return nil
				}
				archived.Add(1)
			}

			return nil
		})
	}

	_ = g.Wait()

	return Result{
		Synced:   int(synced.Load()),
		Skipped:  int(skipped.Load()),
		Failed:   int(failed.Load()),
		Archived: int(archived.Load()),
	}, nil
}

// archiveRepo creates the archive's parent directory before writing to it --
// the destination tree mirrors the forge's namespace, so a repo two levels
// deep needs the same nesting created in the archive directory too.
func archiveRepo(dir, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return Archive(dir, out)
}
