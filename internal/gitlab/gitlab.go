// Package gitlab lists and mirrors repositories from a self-hosted GitLab
// instance over its REST API.
package gitlab

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	backup "github.com/alrayyes/backup-git-repos"
)

const pageSize = 100

// Client lists and mirrors repositories from a self-hosted GitLab instance.
type Client struct {
	BaseURL *url.URL
	Token   string
	HTTP    *http.Client

	// SSHKey, when set, makes Remote return an SSH clone URL and this key
	// instead of an HTTPS one authenticated with Token -- the two are
	// mutually exclusive by construction, enforced in backup.LoadConfig
	// before a Client is even built.
	SSHKey *backup.SSHKey

	// SSHHost overrides the host[:port] Remote clones over SSH from. Empty
	// defaults to BaseURL's own host on the standard port 22 -- see
	// sshCloneURL.
	SSHHost string
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
// git-http operations with a personal access token. With SSHKey set, it
// returns an SSH clone URL and the key instead, with no token involved.
func (c *Client) Remote(r backup.Repo) backup.Remote {
	if c.SSHKey != nil {
		return backup.Remote{
			CloneURL: c.sshCloneURL(r),
			SSHKey:   c.SSHKey,
		}
	}
	return backup.Remote{
		CloneURL:   c.BaseURL.JoinPath(r.Path + ".git").String(),
		AuthHeader: "Basic " + basicAuth("oauth2", c.Token),
	}
}

// sshCloneURL builds the ssh:// clone URL for r. GitLab conventionally
// serves git-over-ssh as the "git" user on the standard port 22 of the same
// host as the web UI, even when the web UI itself runs on a different
// HTTPS port, so that's the default -- SSHHost overrides it for an instance
// that doesn't follow the convention.
func (c *Client) sshCloneURL(r backup.Repo) string {
	host := c.SSHHost
	if host == "" {
		host = c.BaseURL.Hostname()
	}
	u := &url.URL{Scheme: "ssh", User: url.User("git"), Host: host}
	return u.JoinPath(r.Path + ".git").String()
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

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func basicAuth(user, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
}
