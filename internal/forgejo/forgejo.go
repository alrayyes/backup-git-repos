// Package forgejo lists and mirrors repositories from a self-hosted Forgejo
// (or Gitea) instance over its REST API.
package forgejo

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
	"github.com/alrayyes/backup-git-repos/internal/httperr"
)

const pageSize = 50

// Client lists and mirrors repositories from a self-hosted Forgejo instance.
type Client struct {
	BaseURL *url.URL
	Token   string
	HTTP    *http.Client

	// SkipMirrors excludes repositories Forgejo reports as mirrors of an
	// external upstream from ListRepos. Off by default -- the zero value
	// backs up exactly what earlier versions of this client did.
	SkipMirrors bool

	// Logger reports which repository SkipMirrors excluded, so it doesn't
	// just silently vanish from the results. Set via SetLogger, since the
	// composition root builds a Client before it knows which run's logger
	// it should use; nil defaults to slog.Default().
	Logger *slog.Logger
}

// New builds a Client against the given base URL.
func New(base, token string) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse forgejo url: %w", err)
	}

	return &Client{BaseURL: u, Token: token}, nil
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

// Remote implements backup.Remoter by deriving the clone URL from the
// configured base URL rather than the API's own clone_url, which can report
// a host or port the caller can't actually reach -- inside a container it's
// the container's internal port, and behind a reverse proxy it can be wrong
// entirely. It authenticates as HTTP Basic, the token as the password and
// an arbitrary username Forgejo ignores once the password checks out as a
// valid token -- not the "token <t>" scheme Forgejo's own REST API and
// git-http-backend also accept, because its separate LFS batch endpoint
// doesn't: confirmed live against a real Forgejo instance, where syncing an
// LFS-tracked repository authenticated with "token <t>" clones fine but
// 401s the moment Mirror fetches LFS content over the same header. Basic
// works for both, so Remote uses it everywhere rather than special-casing
// LFS.
func (c *Client) Remote(r backup.Repo) backup.Remote {
	return backup.Remote{
		CloneURL:   c.BaseURL.JoinPath(r.Path + ".git").String(),
		AuthHeader: "Basic " + httpauth.Basic("backup-git-repos", c.Token),
	}
}

type searchResponse struct {
	Data []repo `json:"data"`
}

type repo struct {
	FullName string `json:"full_name"`
	Archived bool   `json:"archived"`
	Empty    bool   `json:"empty"`
	Mirror   bool   `json:"mirror"`
}

// ListRepos implements backup.Lister by paging through
// /api/v1/repos/search. The archived parameter on that endpoint is
// tri-state -- true, false, or omitted for all -- so StateAll is a single
// pass rather than two.
func (c *Client) ListRepos(ctx context.Context, state backup.State) ([]backup.Repo, error) {
	var repos []backup.Repo

	for page := 1; ; page++ {
		body, err := c.fetchSearchPage(ctx, page, state)
		if err != nil {
			return nil, err
		}
		if len(body.Data) == 0 {
			break
		}

		for _, r := range body.Data {
			if c.SkipMirrors && r.Mirror {
				c.logger().Info("skipping mirror repository", "path", r.FullName)

				continue
			}
			repos = append(repos, backup.Repo{Path: r.FullName, Archived: r.Archived, Empty: r.Empty})
		}

		if len(body.Data) < pageSize {
			break
		}
	}

	return repos, nil
}

func (c *Client) fetchSearchPage(ctx context.Context, page int, state backup.State) (searchResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.searchURL(page, state).String(), nil)
	if err != nil {
		return searchResponse{}, fmt.Errorf("list forgejo repos: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.Token)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return searchResponse{}, fmt.Errorf("list forgejo repos: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return searchResponse{}, fmt.Errorf("list forgejo repos: %w: %d", httperr.ErrUnexpectedStatus, resp.StatusCode)
	}

	var body searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return searchResponse{}, fmt.Errorf("list forgejo repos: decode response: %w", err)
	}

	return body, nil
}

func (c *Client) searchURL(page int, state backup.State) *url.URL {
	u := c.BaseURL.JoinPath("/api/v1/repos/search")
	q := u.Query()
	q.Set("limit", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	if state != backup.StateAll {
		q.Set("archived", strconv.FormatBool(state == backup.StateArchived))
	}
	u.RawQuery = q.Encode()

	return u
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}

	return http.DefaultClient
}
