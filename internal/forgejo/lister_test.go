//go:build integration

package forgejo_test

import (
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/stretchr/testify/require"
)

func TestListerContract(t *testing.T) {
	t.Parallel()

	f := start(t)
	client, err := forgejo.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	backup.TestLister(t, func(*testing.T) backup.Lister {
		return client
	})
}
