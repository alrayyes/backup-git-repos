//go:build integration

package forgejo_test

import (
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/stretchr/testify/require"
)

func TestReleaseExporterContract(t *testing.T) {
	f := start(t)
	client, err := forgejo.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	backup.TestReleaseExporter(t, func(*testing.T) backup.MetadataExporter {
		return forgejo.NewReleaseExporter(client)
	})
}
