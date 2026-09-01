package github

import (
	"context"
	"fmt"
	"strconv"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
)

// PullRequestExporter exports a repository's pull requests and their review
// comment threads from GitHub.com, using the same authenticated Client
// Lister and Remoter already use.
type PullRequestExporter struct {
	Client *Client
}

// NewPullRequestExporter builds a PullRequestExporter against c.
func NewPullRequestExporter(c *Client) *PullRequestExporter {
	return &PullRequestExporter{Client: c}
}

// Kind implements backup.MetadataExporter.
func (e *PullRequestExporter) Kind() backup.MetadataKind { return backup.MetadataPullRequests }

// ghPullRequest is one item from GET /repos/{owner}/{repo}/pulls -- a
// dedicated endpoint, unlike GitHub's issues endpoint (see issue.go's own
// doc comment for why that one needs to skip items with PullRequest set):
// every item this endpoint returns is already a real pull request, so
// there's nothing to filter out here.
type ghPullRequest struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	User      ghUser     `json:"user"`
	State     string     `json:"state"` // "open" or "closed"
	Head      ghRef      `json:"head"`
	Base      ghRef      `json:"base"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at"`
	MergedAt  *time.Time `json:"merged_at"`
}

// ghRef is a pull request's head or base, only its branch name.
type ghRef struct {
	Ref string `json:"ref"`
}

// ghReviewComment is one item from
// GET /repos/{owner}/{repo}/pulls/{pull_number}/comments -- GitHub's own
// name for a pull request's diff-anchored review comments, confirmed
// against GitHub's REST API reference rather than assumed: Line is the
// comment's line in the current version of the file, null once the diff it
// anchored to has moved on (an outdated comment), which is when
// OriginalLine -- the line it was left on -- is what's still meaningful.
type ghReviewComment struct {
	User         ghUser    `json:"user"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"created_at"`
	Path         string    `json:"path"`
	Line         *int      `json:"line"`
	OriginalLine *int      `json:"original_line"`
}

// Export implements backup.MetadataExporter by paging through
// /repos/{owner}/{repo}/pulls?state=all, fetching each pull request's own
// review comments, and writing every one out with backup.WritePullRequest,
// including a pull request with no comments.
func (e *PullRequestExporter) Export(ctx context.Context, repo backup.Repo, dir string) error {
	for page := 1; ; page++ {
		items, err := e.fetchPullsPage(ctx, repo.Path, page)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}

		for _, it := range items {
			comments, err := e.fetchReviewComments(ctx, repo.Path, it.Number)
			if err != nil {
				return err
			}
			if err := backup.WritePullRequest(dir, toPullRequest(it, comments)); err != nil {
				return fmt.Errorf("write pull request %s#%d: %w", repo.Path, it.Number, err)
			}
		}

		if len(items) < pageSize {
			return nil
		}
	}
}

func (e *PullRequestExporter) fetchPullsPage(ctx context.Context, repoPath string, page int) ([]ghPullRequest, error) {
	u := e.Client.BaseURL.JoinPath("/repos/" + repoPath + "/pulls")
	q := u.Query()
	q.Set("state", "all")
	q.Set("per_page", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	var items []ghPullRequest
	if err := e.Client.getJSON(ctx, u, &items); err != nil {
		return nil, fmt.Errorf("list github pull requests for %s: %w", repoPath, err)
	}

	return items, nil
}

// fetchReviewComments pages through every review comment on pull request
// number -- a single unpaginated request would silently lose every comment
// past GitHub's own default page size (30) once a pull request collects
// more than that, the same reasoning IssueExporter's own fetchComments
// already follows for issue comments.
func (e *PullRequestExporter) fetchReviewComments(ctx context.Context, repoPath string, number int) ([]backup.ReviewComment, error) {
	var all []backup.ReviewComment

	for page := 1; ; page++ {
		u := e.Client.BaseURL.JoinPath("/repos/" + repoPath + "/pulls/" + strconv.Itoa(number) + "/comments")
		q := u.Query()
		q.Set("page", strconv.Itoa(page))
		q.Set("per_page", strconv.Itoa(pageSize))
		u.RawQuery = q.Encode()

		var comments []ghReviewComment
		if err := e.Client.getJSON(ctx, u, &comments); err != nil {
			return nil, fmt.Errorf("list github review comments for %s#%d: %w", repoPath, number, err)
		}
		for _, c := range comments {
			all = append(all, toReviewComment(c))
		}

		if len(comments) < pageSize {
			return all, nil
		}
	}
}

func toPullRequest(it ghPullRequest, comments []backup.ReviewComment) backup.PullRequest {
	state := it.State
	if it.MergedAt != nil {
		state = "merged"
	}

	return backup.PullRequest{
		Number:       it.Number,
		Title:        it.Title,
		Body:         it.Body,
		Author:       it.User.Login,
		State:        state,
		SourceBranch: it.Head.Ref,
		TargetBranch: it.Base.Ref,
		CreatedAt:    it.CreatedAt,
		UpdatedAt:    it.UpdatedAt,
		ClosedAt:     it.ClosedAt,
		MergedAt:     it.MergedAt,
		Comments:     comments,
	}
}

// toReviewComment prefers Line -- the comment's position in the pull
// request's current diff -- and falls back to OriginalLine for a comment
// GitHub itself no longer maps to a current line (see ghReviewComment's own
// doc comment).
func toReviewComment(c ghReviewComment) backup.ReviewComment {
	line := c.Line
	if line == nil {
		line = c.OriginalLine
	}

	out := backup.ReviewComment{Author: c.User.Login, Body: c.Body, CreatedAt: c.CreatedAt, File: c.Path}
	if line != nil {
		out.Line = *line
	}

	return out
}
