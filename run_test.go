package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
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
