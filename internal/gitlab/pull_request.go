package gitlab

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
)

// PullRequestExporter exports a project's merge requests and their review
// comment threads from a self-hosted GitLab instance, using the same
// authenticated Client Lister and Remoter already use.
type PullRequestExporter struct {
	Client *Client
}

// NewPullRequestExporter builds a PullRequestExporter against c.
func NewPullRequestExporter(c *Client) *PullRequestExporter {
	return &PullRequestExporter{Client: c}
}

// Kind implements backup.MetadataExporter.
func (e *PullRequestExporter) Kind() backup.MetadataKind { return backup.MetadataPullRequests }

// glMergeRequest is one item from GET /api/v4/projects/:id/merge_requests.
// Like glIssue, this needs no filtering: merge requests live on their own
// endpoint, entirely separate from issues (see issue.go's own doc comment).
type glMergeRequest struct {
	IID          int        `json:"iid"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Author       glUser     `json:"author"`
	State        string     `json:"state"` // "opened", "closed", "merged", or "locked"
	SourceBranch string     `json:"source_branch"`
	TargetBranch string     `json:"target_branch"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ClosedAt     *time.Time `json:"closed_at"`
	MergedAt     *time.Time `json:"merged_at"`
}

// glDiscussion is one item from
// GET /api/v4/projects/:id/merge_requests/:merge_iid/discussions -- a
// comment thread, one or more notes long. GitLab groups every merge request
// comment into a discussion, whether it's anchored to a diff line or not,
// which is what "review comment threads" (#82's own wording) maps onto most
// directly on this forge.
type glDiscussion struct {
	Notes []glDiscussionNote `json:"notes"`
}

// glDiscussionNote is one note within a glDiscussion. Position is non-nil
// exactly when Type is "DiffNote" -- a comment anchored to a specific file
// and line of the merge request's diff; a plain discussion comment carries
// neither.
type glDiscussionNote struct {
	Type      string      `json:"type"`
	Author    glUser      `json:"author"`
	Body      string      `json:"body"`
	CreatedAt time.Time   `json:"created_at"`
	System    bool        `json:"system"`
	Position  *glPosition `json:"position"`
}

// glPosition is a DiffNote's anchor: the file and line it comments on.
// NewPath/NewLine address the diff's new side, OldPath/OldLine its old side
// -- a comment on a line the merge request removed sets only the latter.
type glPosition struct {
	NewPath string `json:"new_path"`
	NewLine *int   `json:"new_line"`
	OldPath string `json:"old_path"`
	OldLine *int   `json:"old_line"`
}

// Export implements backup.MetadataExporter by paging through
// /api/v4/projects/:id/merge_requests, fetching each merge request's own
// discussions and keeping only the notes a person actually wrote
// (glDiscussionNote.System excludes the rest, the same filter IssueExporter
// already applies to issue notes), and writing every merge request out with
// backup.WritePullRequest, including one with no comments.
func (e *PullRequestExporter) Export(ctx context.Context, repo backup.Repo, dir string) error {
	for page := 1; ; page++ {
		items, next, err := e.fetchMergeRequestsPage(ctx, repo.Path, page)
		if err != nil {
			return err
		}

		for _, it := range items {
			comments, err := e.fetchReviewComments(ctx, repo.Path, it.IID)
			if err != nil {
				return err
			}
			if err := backup.WritePullRequest(dir, toPullRequest(it, comments)); err != nil {
				return fmt.Errorf("write pull request %s!%d: %w", repo.Path, it.IID, err)
			}
		}

		if next == "" {
			return nil
		}
	}
}

func (e *PullRequestExporter) fetchMergeRequestsPage(ctx context.Context, projectPath string, page int) ([]glMergeRequest, string, error) {
	u := e.Client.projectSubURL(projectPath, "merge_requests", page)

	var items []glMergeRequest
	next, err := e.Client.getOptional(ctx, u, "merge requests", projectPath, &items)
	if err != nil {
		return nil, "", fmt.Errorf("list gitlab merge requests for %s: %w", projectPath, err)
	}

	return items, next, nil
}

// fetchReviewComments returns every non-system note across every discussion
// on merge request iid, paging through every result -- a merge request can
// carry more than one page of discussions, the same reasoning
// IssueExporter's own fetchNotes already follows for issue notes.
func (e *PullRequestExporter) fetchReviewComments(ctx context.Context, projectPath string, iid int) ([]backup.ReviewComment, error) {
	var all []backup.ReviewComment

	for page := 1; ; page++ {
		u := e.discussionsURL(projectPath, iid, page)

		var discussions []glDiscussion
		next, err := e.Client.getOptional(ctx, u, "merge request discussions", projectPath, &discussions)
		if err != nil {
			return nil, fmt.Errorf("list gitlab discussions for %s!%d: %w", projectPath, iid, err)
		}

		for _, d := range discussions {
			for _, n := range d.Notes {
				if n.System {
					continue
				}
				all = append(all, toReviewComment(n))
			}
		}

		if next == "" {
			return all, nil
		}
	}
}

func (e *PullRequestExporter) discussionsURL(projectPath string, iid, page int) *url.URL {
	u := e.Client.BaseURL.JoinPath(
		"/api/v4/projects/" + url.PathEscape(projectPath) + "/merge_requests/" + strconv.Itoa(iid) + "/discussions")
	q := u.Query()
	q.Set("per_page", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	return u
}

func toPullRequest(it glMergeRequest, comments []backup.ReviewComment) backup.PullRequest {
	state := it.State
	if state == "opened" {
		state = "open"
	}

	return backup.PullRequest{
		Number:       it.IID,
		Title:        it.Title,
		Body:         it.Description,
		Author:       it.Author.Username,
		State:        state,
		SourceBranch: it.SourceBranch,
		TargetBranch: it.TargetBranch,
		CreatedAt:    it.CreatedAt,
		UpdatedAt:    it.UpdatedAt,
		ClosedAt:     it.ClosedAt,
		MergedAt:     it.MergedAt,
		Comments:     comments,
	}
}

// toReviewComment maps one discussion note, leaving File/Line at their zero
// value for a plain discussion comment (Position is nil) and preferring the
// diff's new side over its old side for one that is anchored.
func toReviewComment(n glDiscussionNote) backup.ReviewComment {
	c := backup.ReviewComment{Author: n.Author.Username, Body: n.Body, CreatedAt: n.CreatedAt}
	if n.Position == nil {
		return c
	}

	c.File = n.Position.NewPath
	if c.File == "" {
		c.File = n.Position.OldPath
	}

	switch {
	case n.Position.NewLine != nil:
		c.Line = *n.Position.NewLine
	case n.Position.OldLine != nil:
		c.Line = *n.Position.OldLine
	}

	return c
}
