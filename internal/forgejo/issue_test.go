//go:build integration

package forgejo_test

import (
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/stretchr/testify/require"
)

func TestIssueExporterContract(t *testing.T) {
	t.Parallel()

	f := start(t)
	client, err := forgejo.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	backup.TestIssueExporter(t, func(*testing.T) backup.MetadataExporter {
		return forgejo.NewIssueExporter(client)
	})
}
