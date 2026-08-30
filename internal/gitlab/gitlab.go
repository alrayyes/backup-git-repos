// Package gitlab lists and mirrors repositories from a self-hosted GitLab
// instance over its REST API.
package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/httpauth"
)

const pageSize = 100

// Client lists and mirrors repositories from a self-hosted GitLab instance.
type Client struct {
	BaseURL *url.URL
	Token   string
	HTTP    *http.Client

	// Logger reports a project whose wiki or snippets came back 403 --
	// GitLab's answer both for the feature being turned off and for the
	// token lacking permission to read it, indistinguishable from the
	// response alone -- so a shorter backup than expected has an answer.
	// Set via SetLogger, since the composition root builds a Client
	// before it knows which run's logger it should use; nil defaults to
	// slog.Default().
	Logger *slog.Logger
}

// SetLogger implements backup.LogSetter.
func (c *Client) SetLogger(l *slog.Logger) {
	c.Logger = l
}

func (c *Client) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// New builds a Client against the given base URL.
func New(base, token string) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse gitlab url: %w", err)
	}
	return &Client{BaseURL: u, Token: token}, nil
}

// Remote implements backup.Remoter by deriving the clone URL from the
// configured base URL rather than the API's own http_url_to_repo, which can
// report a host or port the caller can't actually reach. The oauth2 user
// with the token as password is GitLab's documented form for authenticating
// git-http operations with a personal access token.
func (c *Client) Remote(r backup.Repo) backup.Remote {
	return backup.Remote{
		CloneURL:   c.BaseURL.JoinPath(r.Path + ".git").String(),
		AuthHeader: "Basic " + httpauth.Basic("oauth2", c.Token),
	}
}

type project struct {
	PathWithNamespace string `json:"path_with_namespace"`
	Archived          bool   `json:"archived"`
	EmptyRepo         bool   `json:"empty_repo"`
}

// ListRepos implements backup.Lister by paging through /api/v4/projects.
// simple=true isn't used here even though it would shrink the payload: it
// also drops the archived and empty_repo fields from the response, which
// are exactly the ones this method needs. StateAll makes two passes --
// archived=true and archived=false -- since the endpoint has no "give me
// both" value the way Forgejo's does.
func (c *Client) ListRepos(ctx context.Context, state backup.State) ([]backup.Repo, error) {
	if state == backup.StateAll {
		active, err := c.listByArchived(ctx, false)
		if err != nil {
			return nil, err
		}
		archived, err := c.listByArchived(ctx, true)
		if err != nil {
			return nil, err
		}
		return append(active, archived...), nil
	}

	return c.listByArchived(ctx, state == backup.StateArchived)
}

func (c *Client) listByArchived(ctx context.Context, archived bool) ([]backup.Repo, error) {
	var repos []backup.Repo

	for page := 1; ; page++ {
		projects, next, err := c.fetchProjectsPage(ctx, page, archived)
		if err != nil {
			return nil, err
		}

		for _, p := range projects {
			repos = append(repos, backup.Repo{
				Path:     p.PathWithNamespace,
				Archived: p.Archived,
				Empty:    p.EmptyRepo,
			})

			wiki, err := c.wikiRepo(ctx, p)
			if err != nil {
				return nil, err
			}
			if wiki != nil {
				repos = append(repos, *wiki)
			}

			snippets, err := c.snippetRepos(ctx, p)
			if err != nil {
				return nil, err
			}
			repos = append(repos, snippets...)
		}

		if next == "" {
			break
		}
	}

	return repos, nil
}

// fetchProjectsPage returns the page's projects and the value of the
// x-next-page response header, which is empty on the last page.
func (c *Client) fetchProjectsPage(ctx context.Context, page int, archived bool) ([]project, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.projectsURL(page, archived).String(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("list gitlab projects: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("list gitlab projects: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("list gitlab projects: unexpected status %d", resp.StatusCode)
	}

	var projects []project
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, "", fmt.Errorf("list gitlab projects: decode response: %w", err)
	}

	return projects, resp.Header.Get("x-next-page"), nil
}

func (c *Client) projectsURL(page int, archived bool) *url.URL {
	u := c.BaseURL.JoinPath("/api/v4/projects")
	q := u.Query()
	q.Set("membership", "true")
	q.Set("per_page", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	q.Set("archived", strconv.FormatBool(archived))
	u.RawQuery = q.Encode()
	return u
}

// projectSubURL builds the URL for one page of a per-project sub-resource
// -- wikis, snippets -- addressed by the project's own namespaced path
// rather than its numeric ID, the same URL-encoded-path form GitLab's API
// accepts anywhere a project ":id" is expected.
func (c *Client) projectSubURL(projectPath, sub string, page int) *url.URL {
	u := c.BaseURL.JoinPath("/api/v4/projects/" + url.PathEscape(projectPath) + "/" + sub)
	q := u.Query()
	q.Set("per_page", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()
	return u
}

// getOptional issues an authenticated GET and JSON-decodes a 200 response
// into out, returning the x-next-page response header (empty on the last
// page). A 403 -- GitLab's answer both for a project with the requested
// feature (wiki, snippets) turned off, and for a token that lacks
// permission to read it on this particular project, indistinguishable from
// the response alone -- is treated as success with out left at its zero
// value and no next page, since there's nothing more this client can do
// about either case; resource and projectPath are only for the log line
// that choice produces. Any other non-200 status is an error.
func (c *Client) getOptional(ctx context.Context, u *url.URL, resource, projectPath string, out any) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden {
		c.logger().Info("gitlab project resource not accessible, treating as empty",
			"resource", resource, "project", projectPath)
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return "", err
	}
	return resp.Header.Get("x-next-page"), nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}
