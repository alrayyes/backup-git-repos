//go:build integration && gitlab

package gitlab_test

import (
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
	"github.com/stretchr/testify/require"
)

func TestPullRequestExporterContract(t *testing.T) {
	f := start(t)
	client, err := gitlab.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	backup.TestPullRequestExporter(t, func(*testing.T) backup.MetadataExporter {
		return gitlab.NewPullRequestExporter(client)
	})
}
