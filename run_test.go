package backup_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
		return err
	}
	return os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
}

type fakeRemoter struct{}

func (fakeRemoter) Remote(r backup.Repo) backup.Remote {
	return backup.Remote{CloneURL: "fake://" + r.Path}
}

func TestRun(t *testing.T) {
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
	return ctx.Err()
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
		return ctx.Err()
	}
}

func TestRunDefaultsConcurrencyToNumCPU(t *testing.T) {
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
	for i := 0; i < n; i++ {
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

func TestRunTimeout(t *testing.T) {
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
