//go:build integration

package forgejo_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/stretchr/testify/require"
)

// TestIssueExporterExcludesPullRequests guards #81's own hint: Forgejo's
// issues endpoint returns pull requests alongside real issues unless the
// request asks for type=issues (unlike GitLab, which keeps merge requests
// on an entirely separate endpoint). fixture.seed opens a pull request on
// team/active-repo specifically so a regression here -- the filter dropped,
// or its query param misspelled -- shows up as an extra, wrongly-shaped
// "issue" in the exported set rather than passing unnoticed.
func TestIssueExporterExcludesPullRequests(t *testing.T) {
	t.Parallel()

	f := start(t)
	client, err := forgejo.New(f.BaseURL, f.Token)
	require.NoError(t, err)

	dir := t.TempDir()
	err = forgejo.NewIssueExporter(client).Export(context.Background(), backup.Repo{Path: backup.TestActiveRepoPath}, dir)
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2, "expected exactly the two seeded issues, no pull request among them")

	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		require.NotContains(t, string(data), "a pull request")
	}
}
