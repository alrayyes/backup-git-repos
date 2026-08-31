package backup_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
)

// fakeIssueExporter is an in-memory MetadataExporter for MetadataIssues,
// seeded on TestActiveRepoPath the same way every real forge's issues
// exporter is expected to seed its own fixture repository -- an open issue
// with one comment, and a closed issue with none. Running the contract
// suite against both this and a real forge is what keeps this fake honest,
// the same reasoning fakeLister and fakeMirrorer already follow.
type fakeIssueExporter struct{}

func (fakeIssueExporter) Kind() backup.MetadataKind { return backup.MetadataIssues }

func (fakeIssueExporter) Export(_ context.Context, repo backup.Repo, dir string) error {
	if repo.Path != backup.TestActiveRepoPath {
		return nil
	}

	now := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	if err := backup.WriteIssue(dir, backup.Issue{
		Number: 1, Title: backup.TestIssueOpenTitle, Body: "please fix this", Author: "alice",
		State: "open", Labels: []string{"bug"}, CreatedAt: now, UpdatedAt: now,
		Comments: []backup.Comment{{Author: "bob", Body: backup.TestIssueCommentBody, CreatedAt: now}},
	}); err != nil {
		return fmt.Errorf("write issue 1: %w", err)
	}

	closedAt := now.Add(time.Hour)
	if err := backup.WriteIssue(dir, backup.Issue{
		Number: 2, Title: backup.TestIssueClosedTitle, Body: "already handled", Author: "alice",
		State: "closed", CreatedAt: now, UpdatedAt: closedAt, ClosedAt: &closedAt,
	}); err != nil {
		return fmt.Errorf("write issue 2: %w", err)
	}

	return nil
}

func TestFakeIssueExporter(t *testing.T) {
	backup.TestIssueExporter(t, func(*testing.T) backup.MetadataExporter {
		return fakeIssueExporter{}
	})
}
