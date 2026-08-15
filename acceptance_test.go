//go:build integration

package backup_test

import (
	"context"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

func TestBackupAcceptance(t *testing.T) {
	fixture := startForgejo(t)
	client, err := backup.NewForgejo(fixture.BaseURL, fixture.Token)
	require.NoError(t, err)

	testBackup(t, func(ctx context.Context, opts backup.Options) (backup.Result, error) {
		runner := backup.Runner{
			Lister:   client,
			Mirrorer: backup.Mirror{},
			Remoter:  client,
		}
		return runner.Run(ctx, opts)
	})
}
