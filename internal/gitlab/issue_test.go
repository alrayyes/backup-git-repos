//go:build integration && gitlab

package gitlab_test

import (
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
	"github.com/stretchr/testify/require"
)

// No t.Parallel() here: start(t) boots its own real GitLab CE container,
// and this package's container-booting tests running concurrently would
// mean several full instances at once on whatever's running the nightly
// lane -- see mirror_lfs_test.go's own tests for the same reasoning.
func TestIssueExporterContract(t *testing.T) {
	f := start(t)
	client, err := gitlab.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	backup.TestIssueExporter(t, func(*testing.T) backup.MetadataExporter {
		return gitlab.NewIssueExporter(client)
	})
}
