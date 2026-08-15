//go:build integration

package forgejo_test

import (
	"context"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/stretchr/testify/require"
)

func TestBackupAcceptance(t *testing.T) {
	f := start(t)
	client, err := forgejo.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	backup.TestBackup(t, func(ctx context.Context, opts backup.Options) (backup.Result, error) {
		runner := backup.Runner{
			Lister:   client,
			Mirrorer: backup.Mirror{},
			Remoter:  client,
		}
		return runner.Run(ctx, opts)
	})
}
