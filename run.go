package backup

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync/atomic"

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

// Options configures a Run.
type Options struct {
	Dest        string
	State       State
	Concurrency int
	Log         *slog.Logger
}

// Result reports what a Run did.
type Result struct {
	Synced  int
	Skipped int
	Failed  int
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
		concurrency = 1
	}

	var synced, skipped, failed atomic.Int64

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for _, repo := range repos {
		if repo.Empty {
			log.Warn("skipping empty repository", "path", repo.Path)
			skipped.Add(1)
			continue
		}

		g.Go(func() error {
			dir := filepath.Join(opts.Dest, filepath.FromSlash(repo.Path)+".git")
			if err := r.Mirrorer.Sync(ctx, r.Remoter.Remote(repo), dir); err != nil {
				log.Error("mirror failed", "path", repo.Path, "error", err)
				failed.Add(1)
				return nil
			}
			synced.Add(1)
			return nil
		})
	}

	_ = g.Wait()

	return Result{
		Synced:  int(synced.Load()),
		Skipped: int(skipped.Load()),
		Failed:  int(failed.Load()),
	}, nil
}
