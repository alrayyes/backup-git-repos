// Package github lists and mirrors repositories from GitHub.com over its
// REST API.
package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	backup "github.com/alrayyes/backup-git-repos"
)

// ErrMissingRepoScope means a classic personal access token's granted
// scopes don't include "repo". GitHub doesn't error on that -- /user/repos
// still returns 200 with just the public repositories -- so left unchecked
// this is a backup that silently never carried anything private.
var ErrMissingRepoScope = errors.New(`github token is missing the "repo" scope: private repositories would not be listed`)

const (
	// defaultBaseURL is GitHub.com's own REST API host. A test points
	// Client.BaseURL at a recorded fixture server instead.
	defaultBaseURL = "https://api.github.com"

	// apiVersion is the REST API version pinned in every request, so a
	// GitHub-side default change never reshapes a response underneath
	// this adapter without a deliberate bump here.
	apiVersion = "2022-11-28"

	pageSize = 100
)

// cloneHost is where GitHub.com serves git-over-HTTPS from. It's a fixed
// value, never Client.BaseURL: BaseURL is the API host, and GitHub.com
// serves the API from a different domain (api.github.com) than the one it
// serves git and the web UI from (github.com).
var cloneHost = &url.URL{Scheme: "https", Host: "github.com"}

// Client lists and mirrors repositories from GitHub.com.
type Client struct {
	BaseURL *url.URL
	Token   string
	HTTP    *http.Client
}

// New builds a Client. An empty base uses GitHub.com's own API host; a test
// points it at a recorded fixture server instead.
func New(base, token string) (*Client, error) {
	if base == "" {
		base = defaultBaseURL
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse github url: %w", err)
	}
	return &Client{BaseURL: u, Token: token}, nil
}

// Remote implements backup.Remoter. GitHub accepts a personal access token
// as the Basic auth username with an empty password, the same form as
// `git clone https://<token>@github.com/owner/repo.git`.
func (c *Client) Remote(r backup.Repo) backup.Remote {
	return backup.Remote{
		CloneURL:   cloneHost.JoinPath(r.Path + ".git").String(),
		AuthHeader: "Basic " + basicAuth(c.Token, ""),
	}
}

type repo struct {
	FullName string `json:"full_name"`
	Archived bool   `json:"archived"`
	Size     int    `json:"size"`
}

// ListRepos implements backup.Lister by paging through GET /user/repos for
// the authenticated token. That endpoint has no server-side archived
// filter, unlike Forgejo's search or GitLab's projects endpoint, so every
// repo is fetched once and filtered by state locally. GitHub's repo object
// carries no "is this repo empty" field either; size is reported in
// kilobytes and a repo with no commits pushed yet reports 0, which is what
// Empty is derived from.
func (c *Client) ListRepos(ctx context.Context, state backup.State) ([]backup.Repo, error) {
	var repos []backup.Repo

	for page := 1; ; page++ {
		items, scopes, err := c.fetchReposPage(ctx, page)
		if err != nil {
			return nil, err
		}
		if page == 1 {
			if err := checkRepoScope(scopes); err != nil {
				return nil, err
			}
		}
		if len(items) == 0 {
			break
		}

		for _, r := range items {
			if !wantsState(state, r.Archived) {
				continue
			}
			repos = append(repos, backup.Repo{Path: r.FullName, Archived: r.Archived, Empty: r.Size == 0})
		}

		if len(items) < pageSize {
			break
		}
	}

	return repos, nil
}

func wantsState(state backup.State, archived bool) bool {
	switch state {
	case backup.StateActive:
		return !archived
	case backup.StateArchived:
		return archived
	default:
		return true
	}
}

// fetchReposPage returns the page's repos alongside the raw
// X-OAuth-Scopes response header, which the caller checks once against
// page 1 -- GitHub sends it on every request from a classic personal
// access token, so any page would do, but checking early fails a scope
// problem before more pages are even fetched.
func (c *Client) fetchReposPage(ctx context.Context, page int) ([]repo, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.reposURL(page).String(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("list github repos: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("list github repos: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("list github repos: unexpected status %d", resp.StatusCode)
	}

	var items []repo
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, "", fmt.Errorf("list github repos: decode response: %w", err)
	}
	return items, resp.Header.Get("X-OAuth-Scopes"), nil
}

// checkRepoScope fails on a classic token's scope list that's missing
// "repo". GitHub omits the X-OAuth-Scopes header entirely for a
// fine-grained token, so an empty raw value isn't itself an error -- there's
// no list to check against, and fine-grained access is scoped a different
// way (per-repository, at token creation).
func checkRepoScope(raw string) error {
	if raw == "" {
		return nil
	}
	for _, s := range strings.Split(raw, ",") {
		if strings.TrimSpace(s) == "repo" {
			return nil
		}
	}
	return ErrMissingRepoScope
}

func (c *Client) reposURL(page int) *url.URL {
	u := c.BaseURL.JoinPath("/user/repos")
	q := u.Query()
	q.Set("affiliation", "owner,collaborator,organization_member")
	q.Set("per_page", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
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
