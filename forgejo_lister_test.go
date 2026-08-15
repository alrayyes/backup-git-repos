//go:build integration

package backup_test

import (
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

func TestForgejoListerContract(t *testing.T) {
	fixture := startForgejo(t)
	lister, err := backup.NewForgejo(fixture.BaseURL, fixture.Token)
	require.NoError(t, err)

	testLister(t, func(*testing.T) backup.Lister {
		return lister
	})
}
