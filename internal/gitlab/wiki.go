package gitlab

import (
	"context"
	"fmt"

	backup "github.com/alrayyes/backup-git-repos"
)

// wikiPage is an item from GET /api/v4/projects/:id/wikis. Only its
// presence in the list matters here, not its content, so slug is decoded
// purely so the response isn't discarded unread.
type wikiPage struct {
	Slug string `json:"slug"`
}

// wikiRepo returns the Repo for project p's wiki, or nil if the project
// carries no wiki content -- an empty page list, or the wiki feature
// turned off entirely (GitLab answers 403 for a project with wikis
// disabled, which getOptional treats the same as an empty list: either
// way there's nothing to back up). Only the first page of results is
// fetched: a non-empty first page already proves the wiki has content,
// which is all this needs to know -- unlike snippetRepos, it doesn't have
// to enumerate every page to mirror every item. The wiki is its own git
// repository, cloneable at the project's own path with ".wiki" appended
// before ".git" -- confirmed against a live GitLab CE container rather
// than assumed from the API docs.
func (c *Client) wikiRepo(ctx context.Context, p project) (*backup.Repo, error) {
	var pages []wikiPage
	u := c.projectSubURL(p.PathWithNamespace, "wikis", 1)
	if _, err := c.getOptional(ctx, u, "wiki", p.PathWithNamespace, &pages); err != nil {
		return nil, fmt.Errorf("list gitlab wiki pages for %s: %w", p.PathWithNamespace, err)
	}
	if len(pages) == 0 {
		return nil, nil
	}

	return &backup.Repo{
		Path:     p.PathWithNamespace + ".wiki",
		Archived: p.Archived,
	}, nil
}
