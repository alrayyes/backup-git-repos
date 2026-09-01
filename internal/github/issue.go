package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
)

// IssueExporter exports a repository's issues and their comments from
// GitHub.com, using the same authenticated Client Lister and Remoter
// already use.
type IssueExporter struct {
	Client *Client
}

// NewIssueExporter builds an IssueExporter against c.
func NewIssueExporter(c *Client) *IssueExporter {
	return &IssueExporter{Client: c}
}

// Kind implements backup.MetadataExporter.
func (e *IssueExporter) Kind() backup.MetadataKind { return backup.MetadataIssues }

// ghIssue is one item from GET /repos/{owner}/{repo}/issues. PullRequest is
// non-nil exactly when this item is actually a pull request -- GitHub's
// issues endpoint returns pull requests alongside real issues, with no
// server-side filter to exclude them the way GitLab's separate merge
// requests endpoint makes unnecessary there.
type ghIssue struct {
	Number      int              `json:"number"`
	Title       string           `json:"title"`
	Body        string           `json:"body"`
	User        ghUser           `json:"user"`
	State       string           `json:"state"`
	Labels      []ghLabel        `json:"labels"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	ClosedAt    *time.Time       `json:"closed_at"`
	PullRequest *json.RawMessage `json:"pull_request,omitempty"`
}

type ghUser struct {
	Login string `json:"login"`
}

type ghLabel struct {
	Name string `json:"name"`
}

// ghComment is one item from
// GET /repos/{owner}/{repo}/issues/{issue_number}/comments.
type ghComment struct {
	User      ghUser    `json:"user"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Export implements backup.MetadataExporter by paging through
// /repos/{owner}/{repo}/issues?state=all, skipping every item whose
// PullRequest field is set -- see ghIssue's own doc comment -- fetching
// each remaining issue's own comments, and writing every one out with
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
			if err := e.exportIssue(ctx, repo, dir, it); err != nil {
				return err
			}
		}

		if len(items) < pageSize {
			return nil
		}
	}
}

// exportIssue fetches its comments and writes it out to dir, skipping
// pull requests entirely -- see ghIssue's own doc comment.
func (e *IssueExporter) exportIssue(ctx context.Context, repo backup.Repo, dir string, it ghIssue) error {
	if it.PullRequest != nil {
		return nil
	}

	comments, err := e.fetchComments(ctx, repo.Path, it.Number)
	if err != nil {
		return err
	}

	if err := backup.WriteIssue(dir, toIssue(it, comments)); err != nil {
		return fmt.Errorf("write issue %s#%d: %w", repo.Path, it.Number, err)
	}

	return nil
}

func (e *IssueExporter) fetchIssuesPage(ctx context.Context, repoPath string, page int) ([]ghIssue, error) {
	u := e.Client.BaseURL.JoinPath("/repos/" + repoPath + "/issues")
	q := u.Query()
	q.Set("state", "all")
	q.Set("per_page", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	var items []ghIssue
	if err := getJSON(ctx, e.Client, u, &items); err != nil {
		return nil, fmt.Errorf("list github issues for %s: %w", repoPath, err)
	}

	return items, nil
}

// fetchComments pages through every comment on issue number -- a single
// unpaginated request would silently lose every comment past GitHub's own
// default page size (30) once an issue collects more than that.
func (e *IssueExporter) fetchComments(ctx context.Context, repoPath string, number int) ([]ghComment, error) {
	var all []ghComment

	for page := 1; ; page++ {
		u := e.Client.BaseURL.JoinPath("/repos/" + repoPath + "/issues/" + strconv.Itoa(number) + "/comments")
		q := u.Query()
		q.Set("page", strconv.Itoa(page))
		q.Set("per_page", strconv.Itoa(pageSize))
		u.RawQuery = q.Encode()

		var comments []ghComment
		if err := getJSON(ctx, e.Client, u, &comments); err != nil {
			return nil, fmt.Errorf("list github comments for %s#%d: %w", repoPath, number, err)
		}
		all = append(all, comments...)

		if len(comments) < pageSize {
			return all, nil
		}
	}
}

func toIssue(it ghIssue, comments []ghComment) backup.Issue {
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
