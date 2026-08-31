package gitlab

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
)

// IssueExporter exports a project's issues and their comments from a
// self-hosted GitLab instance, using the same authenticated Client Lister
// and Remoter already use.
type IssueExporter struct {
	Client *Client
}

// NewIssueExporter builds an IssueExporter against c.
func NewIssueExporter(c *Client) *IssueExporter {
	return &IssueExporter{Client: c}
}

// Kind implements backup.MetadataExporter.
func (e *IssueExporter) Kind() backup.MetadataKind { return backup.MetadataIssues }

// glIssue is one item from GET /api/v4/projects/:id/issues. Unlike Forgejo
// and GitHub, GitLab has no need to filter merge requests out of this list:
// merge requests live on their own, entirely separate
// /api/v4/projects/:id/merge_requests endpoint, so every item this one
// returns is already a real issue.
type glIssue struct {
	IID         int        `json:"iid"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Author      glUser     `json:"author"`
	State       string     `json:"state"` // "opened" or "closed"
	Labels      []string   `json:"labels"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
}

type glUser struct {
	Username string `json:"username"`
}

// glNote is one item from
// GET /api/v4/projects/:id/issues/:issue_iid/notes. System is true for a
// note GitLab generated itself -- "changed the description", "assigned to
// @alice" -- rather than something a person actually wrote, and those are
// filtered out: they're activity log entries, not comments.
type glNote struct {
	Author    glUser    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	System    bool      `json:"system"`
}

// Export implements backup.MetadataExporter by paging through
// /api/v4/projects/:id/issues, fetching each issue's own notes and keeping
// only the ones a person actually wrote (glNote.System excludes the rest),
// and writing every issue out with backup.WriteIssue, including one with no
// comments. "opened" is normalized to "open" so backup.Issue.State reads
// the same regardless of which forge produced it.
func (e *IssueExporter) Export(ctx context.Context, repo backup.Repo, dir string) error {
	for page := 1; ; page++ {
		items, next, err := e.fetchIssuesPage(ctx, repo.Path, page)
		if err != nil {
			return err
		}

		for _, it := range items {
			notes, err := e.fetchNotes(ctx, repo.Path, it.IID)
			if err != nil {
				return err
			}
			if err := backup.WriteIssue(dir, toIssue(it, notes)); err != nil {
				return fmt.Errorf("write issue %s#%d: %w", repo.Path, it.IID, err)
			}
		}

		if next == "" {
			return nil
		}
	}
}

func (e *IssueExporter) fetchIssuesPage(ctx context.Context, projectPath string, page int) ([]glIssue, string, error) {
	u := e.Client.projectSubURL(projectPath, "issues", page)

	var items []glIssue
	next, err := e.Client.getOptional(ctx, u, "issues", projectPath, &items)
	if err != nil {
		return nil, "", fmt.Errorf("list gitlab issues for %s: %w", projectPath, err)
	}

	return items, next, nil
}

// fetchNotes returns every non-system note on issue iid, paging through
// every result -- an issue can carry more than one page of comments, unlike
// wikiRepo's own single-page check.
func (e *IssueExporter) fetchNotes(ctx context.Context, projectPath string, iid int) ([]glNote, error) {
	var all []glNote

	for page := 1; ; page++ {
		u := e.notesURL(projectPath, iid, page)

		var notes []glNote
		next, err := e.Client.getOptional(ctx, u, "issue notes", projectPath, &notes)
		if err != nil {
			return nil, fmt.Errorf("list gitlab notes for %s!%d: %w", projectPath, iid, err)
		}

		for _, n := range notes {
			if n.System {
				continue
			}
			all = append(all, n)
		}

		if next == "" {
			return all, nil
		}
	}
}

func (e *IssueExporter) notesURL(projectPath string, iid, page int) *url.URL {
	u := e.Client.BaseURL.JoinPath(
		"/api/v4/projects/" + url.PathEscape(projectPath) + "/issues/" + strconv.Itoa(iid) + "/notes")
	q := u.Query()
	q.Set("per_page", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	return u
}

func toIssue(it glIssue, notes []glNote) backup.Issue {
	state := it.State
	if state == "opened" {
		state = "open"
	}

	out := backup.Issue{
		Number:    it.IID,
		Title:     it.Title,
		Body:      it.Description,
		Author:    it.Author.Username,
		State:     state,
		Labels:    it.Labels,
		CreatedAt: it.CreatedAt,
		UpdatedAt: it.UpdatedAt,
		ClosedAt:  it.ClosedAt,
		Comments:  make([]backup.Comment, len(notes)),
	}
	for i, n := range notes {
		out.Comments[i] = backup.Comment{Author: n.Author.Username, Body: n.Body, CreatedAt: n.CreatedAt}
	}

	return out
}
