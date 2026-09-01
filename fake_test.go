package backup_test

import (
	"context"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
)

// fakeLister is an in-memory Lister seeded with the same fixture set every
// adapter's own contract test seeds: one active repo, one archived, one
// empty. Running the contract suite against both is what keeps this fake
// honest.
type fakeLister struct {
	repos []backup.Repo
}

func newFakeLister() *fakeLister {
	return &fakeLister{repos: []backup.Repo{
		{Path: backup.TestActiveRepoPath, Archived: false, Empty: false},
		{Path: backup.TestArchivedRepoPath, Archived: true, Empty: false},
		{Path: backup.TestEmptyRepoPath, Archived: false, Empty: true},
	}}
}

func (f *fakeLister) ListRepos(_ context.Context, state backup.State) ([]backup.Repo, error) {
	if state == backup.StateAll {
		return f.repos, nil
	}

	var out []backup.Repo
	for _, r := range f.repos {
		if r.Archived == (state == backup.StateArchived) {
			out = append(out, r)
		}
	}

	return out, nil
}

func TestFakeLister(t *testing.T) {
	t.Parallel()

	backup.TestLister(t, func(*testing.T) backup.Lister {
		return newFakeLister()
	})
}
