package gitlab

import (
	"context"
	"fmt"
	"strconv"

	backup "github.com/alrayyes/backup-git-repos"
)

// snippet is an item from GET /api/v4/projects/:id/snippets. Only the ID
// is decoded: the clone URL isn't taken from the response's own
// http_url_to_repo, which can report a host or port the caller can't
// reach -- the same reasoning Client.Remote already applies to projects --
// it's derived instead, in snippetRepos below.
type snippet struct {
	ID int `json:"id"`
}

// snippetRepos returns the Repo for each of project p's snippets, paging
// through every result rather than just the first -- unlike wikiRepo,
// which only needs to know a wiki has content, this has to enumerate every
// snippet to mirror each one, and GitLab's default page size (20) is well
// under what a project can hold. A project with none returns an empty
// slice, not an error -- that's the "no empty entry" case: nothing is
// appended to the caller's repo list. Each snippet's own git repository
// clones at the project's own path with "/snippets/<id>" appended, which
// is exactly what a live GitLab CE container's http_url_to_repo reports
// for a project snippet -- confirmed directly rather than assumed, since
// the ID needed no separate lookup.
func (c *Client) snippetRepos(ctx context.Context, p project) ([]backup.Repo, error) {
	var repos []backup.Repo

	for page := 1; ; page++ {
		var snippets []snippet
		u := c.projectSubURL(p.PathWithNamespace, "snippets", page)
		next, err := c.getOptional(ctx, u, "snippets", p.PathWithNamespace, &snippets)
		if err != nil {
			return nil, fmt.Errorf("list gitlab snippets for %s: %w", p.PathWithNamespace, err)
		}

		for _, s := range snippets {
			repos = append(repos, backup.Repo{
				Path:     p.PathWithNamespace + "/snippets/" + strconv.Itoa(s.ID),
				Archived: p.Archived,
			})
		}

		if next == "" {
			break
		}
	}

	return repos, nil
}
