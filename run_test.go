package backup_test

import (
	"context"
	"os"
	"path/filepath"
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
	testBackup(t, func(ctx context.Context, opts backup.Options) (backup.Result, error) {
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
