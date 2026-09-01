package forgejo

import (
	"context"
	"fmt"
	"strconv"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
)

// PullRequestExporter exports a repository's pull requests and their review
// comment threads from a self-hosted Forgejo (or Gitea) instance, over the
// same REST API and the same authenticated Client Lister and Remoter
// already use.
type PullRequestExporter struct {
	Client *Client
}

// NewPullRequestExporter builds a PullRequestExporter against c.
func NewPullRequestExporter(c *Client) *PullRequestExporter {
	return &PullRequestExporter{Client: c}
}

// Kind implements backup.MetadataExporter.
func (e *PullRequestExporter) Kind() backup.MetadataKind { return backup.MetadataPullRequests }

// forgePullRequest is one item from GET /repos/{owner}/{repo}/pulls -- a
// dedicated endpoint, unlike Forgejo's issues endpoint (see issue.go's own
// doc comment for why that one needs type=issues to exclude pull requests):
// every item this endpoint returns is already a real pull request, so
// there's nothing to filter out here.
type forgePullRequest struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	User      forgeUser  `json:"user"`
	State     string     `json:"state"` // "open" or "closed"
	Merged    bool       `json:"merged"`
	Head      forgeRef   `json:"head"`
	Base      forgeRef   `json:"base"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at"`
	MergedAt  *time.Time `json:"merged_at"`
}

// forgeRef is a pull request's head or base, only its branch name.
type forgeRef struct {
	Ref string `json:"ref"`
}

// forgeReview is one item from
// GET /repos/{owner}/{repo}/pulls/{index}/reviews -- one reviewer's
// submission, carrying its own overall Body (a review's "LGTM", say) on top
// of whatever line-anchored comments it left, fetched separately with
// fetchOneReviewComments.
type forgeReview struct {
	ID          int64     `json:"id"`
	User        forgeUser `json:"user"`
	Body        string    `json:"body"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// forgeReviewComment is one item from
// GET /repos/{owner}/{repo}/pulls/{index}/reviews/{id}/comments. Position is
// the JSON field name, confirmed against Forgejo's own swagger definition
// (x-go-name "LineNum") rather than assumed from the name alone: despite the
// historical "position" name, it's the new file's actual line number the
// comment anchors to, 0 when the comment only anchors to the old side of the
// diff (OriginalPosition, "OldLineNum" in the same swagger) instead.
type forgeReviewComment struct {
	User             forgeUser `json:"user"`
	Body             string    `json:"body"`
	CreatedAt        time.Time `json:"created_at"`
	Path             string    `json:"path"`
	Position         int       `json:"position"`
	OriginalPosition int       `json:"original_position"`
}

// Export implements backup.MetadataExporter by paging through
// /repos/{owner}/{repo}/pulls?state=all, fetching each pull request's own
// review comment threads -- every review left on it, its own overall body
// when non-empty plus each of its line-anchored comments -- and writing
// every one out with backup.WritePullRequest, including a pull request with
// no comments.
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

func (e *PullRequestExporter) fetchPullsPage(ctx context.Context, repoPath string, page int) ([]forgePullRequest, error) {
	u := e.Client.BaseURL.JoinPath("/api/v1/repos/" + repoPath + "/pulls")
	q := u.Query()
	q.Set("state", "all")
	q.Set("limit", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	var items []forgePullRequest
	if err := e.Client.getJSON(ctx, u, &items); err != nil {
		return nil, fmt.Errorf("list forgejo pull requests for %s: %w", repoPath, err)
	}

	return items, nil
}

// fetchReviewComments returns every review comment thread on pull request
// number: each review's own overall body, when a reviewer left one, plus
// every line-anchored comment that review carries. The reviews list itself
// pages the same way issues and their comments do; each individual review's
// own comments endpoint doesn't paginate, so one request each covers it.
func (e *PullRequestExporter) fetchReviewComments(ctx context.Context, repoPath string, number int) ([]backup.ReviewComment, error) {
	var all []backup.ReviewComment

	for page := 1; ; page++ {
		reviews, err := e.fetchReviewsPage(ctx, repoPath, number, page)
		if err != nil {
			return nil, err
		}

		for _, r := range reviews {
			if r.Body != "" {
				all = append(all, backup.ReviewComment{Author: r.User.Login, Body: r.Body, CreatedAt: r.SubmittedAt})
			}

			comments, err := e.fetchOneReviewComments(ctx, repoPath, number, r.ID)
			if err != nil {
				return nil, err
			}
			all = append(all, comments...)
		}

		if len(reviews) < pageSize {
			return all, nil
		}
	}
}

func (e *PullRequestExporter) fetchReviewsPage(ctx context.Context, repoPath string, number, page int) ([]forgeReview, error) {
	u := e.Client.BaseURL.JoinPath("/api/v1/repos/" + repoPath + "/pulls/" + strconv.Itoa(number) + "/reviews")
	q := u.Query()
	q.Set("limit", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	var reviews []forgeReview
	if err := e.Client.getJSON(ctx, u, &reviews); err != nil {
		return nil, fmt.Errorf("list forgejo reviews for %s#%d: %w", repoPath, number, err)
	}

	return reviews, nil
}

func (e *PullRequestExporter) fetchOneReviewComments(
	ctx context.Context, repoPath string, number int, reviewID int64,
) ([]backup.ReviewComment, error) {
	u := e.Client.BaseURL.JoinPath(
		"/api/v1/repos/" + repoPath + "/pulls/" + strconv.Itoa(number) +
			"/reviews/" + strconv.FormatInt(reviewID, 10) + "/comments")

	var comments []forgeReviewComment
	if err := e.Client.getJSON(ctx, u, &comments); err != nil {
		return nil, fmt.Errorf("list forgejo review comments for %s#%d review %d: %w", repoPath, number, reviewID, err)
	}

	out := make([]backup.ReviewComment, len(comments))
	for i, c := range comments {
		out[i] = toReviewComment(c)
	}

	return out, nil
}

func toPullRequest(it forgePullRequest, comments []backup.ReviewComment) backup.PullRequest {
	state := it.State
	if it.Merged {
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

// toReviewComment maps one line-anchored review comment, preferring the new
// side of the diff (Position) and falling back to the old side
// (OriginalPosition) for a comment left on a line the pull request itself
// removed.
func toReviewComment(c forgeReviewComment) backup.ReviewComment {
	line := c.Position
	if line == 0 {
		line = c.OriginalPosition
	}

	return backup.ReviewComment{Author: c.User.Login, Body: c.Body, CreatedAt: c.CreatedAt, File: c.Path, Line: line}
}
