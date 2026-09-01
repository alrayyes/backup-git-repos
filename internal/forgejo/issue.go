package forgejo

import (
	"context"
	"fmt"
	"strconv"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
)

// IssueExporter exports a repository's issues and their comments from a
// self-hosted Forgejo (or Gitea) instance, over the same REST API and the
// same authenticated Client Lister and Remoter already use.
type IssueExporter struct {
	Client *Client
}

// NewIssueExporter builds an IssueExporter against c.
func NewIssueExporter(c *Client) *IssueExporter {
	return &IssueExporter{Client: c}
}

// Kind implements backup.MetadataExporter.
func (e *IssueExporter) Kind() backup.MetadataKind { return backup.MetadataIssues }

// forgeIssue is one item from GET /repos/{owner}/{repo}/issues.
type forgeIssue struct {
	Number    int          `json:"number"`
	Title     string       `json:"title"`
	Body      string       `json:"body"`
	User      forgeUser    `json:"user"`
	State     string       `json:"state"`
	Labels    []forgeLabel `json:"labels"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	ClosedAt  *time.Time   `json:"closed_at"`
}

type forgeUser struct {
	Login string `json:"login"`
}

type forgeLabel struct {
	Name string `json:"name"`
}

// forgeComment is one item from
// GET /repos/{owner}/{repo}/issues/{index}/comments.
type forgeComment struct {
	User      forgeUser `json:"user"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Export implements backup.MetadataExporter by paging through
// /repos/{owner}/{repo}/issues?type=issues&state=all -- type=issues is what
// excludes pull requests, which this same endpoint otherwise returns
// alongside real issues (Forgejo's API follows GitHub's shape here, not
// GitLab's, which keeps merge requests on an entirely separate endpoint) --
// fetching each issue's own comments and writing every one out with
// backup.WriteIssue, including an issue with no comments.
func (e *IssueExporter) Export(ctx context.Context, repo backup.Repo, dir string) error {
	for page := 1; ; page++ {
		items, err := e.fetchIssuesPage(ctx, repo.Path, page)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}

		for _, it := range items {
			comments, err := e.fetchComments(ctx, repo.Path, it.Number)
			if err != nil {
				return err
			}
			if err := backup.WriteIssue(dir, toIssue(it, comments)); err != nil {
				return fmt.Errorf("write issue %s#%d: %w", repo.Path, it.Number, err)
			}
		}

		if len(items) < pageSize {
			return nil
		}
	}
}

func (e *IssueExporter) fetchIssuesPage(ctx context.Context, repoPath string, page int) ([]forgeIssue, error) {
	u := e.Client.BaseURL.JoinPath("/api/v1/repos/" + repoPath + "/issues")
	q := u.Query()
	q.Set("type", "issues")
	q.Set("state", "all")
	q.Set("limit", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	var items []forgeIssue
	if err := getJSON(ctx, e.Client, u, &items); err != nil {
		return nil, fmt.Errorf("list forgejo issues for %s: %w", repoPath, err)
	}

	return items, nil
}

// fetchComments pages through every comment on issue number -- a single
// unpaginated request would silently lose every comment past the server's
// own default page size once an issue collects more than that.
func (e *IssueExporter) fetchComments(ctx context.Context, repoPath string, number int) ([]forgeComment, error) {
	var all []forgeComment

	for page := 1; ; page++ {
		u := e.Client.BaseURL.JoinPath("/api/v1/repos/" + repoPath + "/issues/" + strconv.Itoa(number) + "/comments")
		q := u.Query()
		q.Set("page", strconv.Itoa(page))
		q.Set("limit", strconv.Itoa(pageSize))
		u.RawQuery = q.Encode()

		var comments []forgeComment
		if err := getJSON(ctx, e.Client, u, &comments); err != nil {
			return nil, fmt.Errorf("list forgejo comments for %s#%d: %w", repoPath, number, err)
		}
		all = append(all, comments...)

		if len(comments) < pageSize {
			return all, nil
		}
	}
}

func toIssue(it forgeIssue, comments []forgeComment) backup.Issue {
	labels := make([]string, len(it.Labels))
	for i, l := range it.Labels {
		labels[i] = l.Name
	}

	out := backup.Issue{
		Number:    it.Number,
		Title:     it.Title,
		Body:      it.Body,
		Author:    it.User.Login,
		State:     it.State,
		Labels:    labels,
		CreatedAt: it.CreatedAt,
		UpdatedAt: it.UpdatedAt,
		ClosedAt:  it.ClosedAt,
		Comments:  make([]backup.Comment, len(comments)),
	}
	for i, c := range comments {
		out.Comments[i] = backup.Comment{Author: c.User.Login, Body: c.Body, CreatedAt: c.CreatedAt}
	}

	return out
}
