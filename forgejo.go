package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const forgejoPageSize = 50

// Forgejo lists repositories from a self-hosted Forgejo (or Gitea) instance
// over its REST API.
type Forgejo struct {
	BaseURL *url.URL
	Token   string
	HTTP    *http.Client
}

// NewForgejo builds a Forgejo client against the given base URL.
func NewForgejo(base, token string) (*Forgejo, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse forgejo url: %w", err)
	}
	return &Forgejo{BaseURL: u, Token: token}, nil
}

type forgejoSearchResponse struct {
	Data []forgejoRepo `json:"data"`
}

type forgejoRepo struct {
	FullName string `json:"full_name"`
	Archived bool   `json:"archived"`
	Empty    bool   `json:"empty"`
}

// ListRepos implements Lister by paging through /api/v1/repos/search. The
// archived parameter on that endpoint is tri-state -- true, false, or
// omitted for all -- so StateAll is a single pass rather than two.
func (f *Forgejo) ListRepos(ctx context.Context, state State) ([]Repo, error) {
	var repos []Repo

	for page := 1; ; page++ {
		body, err := f.fetchSearchPage(ctx, page, state)
		if err != nil {
			return nil, err
		}
		if len(body.Data) == 0 {
			break
		}

		for _, r := range body.Data {
			repos = append(repos, Repo{Path: r.FullName, Archived: r.Archived, Empty: r.Empty})
		}

		if len(body.Data) < forgejoPageSize {
			break
		}
	}

	return repos, nil
}

func (f *Forgejo) fetchSearchPage(ctx context.Context, page int, state State) (forgejoSearchResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.searchURL(page, state).String(), nil)
	if err != nil {
		return forgejoSearchResponse{}, fmt.Errorf("list forgejo repos: %w", err)
	}
	req.Header.Set("Authorization", "token "+f.Token)

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return forgejoSearchResponse{}, fmt.Errorf("list forgejo repos: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return forgejoSearchResponse{}, fmt.Errorf("list forgejo repos: unexpected status %d", resp.StatusCode)
	}

	var body forgejoSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return forgejoSearchResponse{}, fmt.Errorf("list forgejo repos: decode response: %w", err)
	}
	return body, nil
}

func (f *Forgejo) searchURL(page int, state State) *url.URL {
	u := f.BaseURL.JoinPath("/api/v1/repos/search")
	q := u.Query()
	q.Set("limit", strconv.Itoa(forgejoPageSize))
	q.Set("page", strconv.Itoa(page))
	if state != StateAll {
		q.Set("archived", strconv.FormatBool(state == StateArchived))
	}
	u.RawQuery = q.Encode()
	return u
}

func (f *Forgejo) httpClient() *http.Client {
	if f.HTTP != nil {
		return f.HTTP
	}
	return http.DefaultClient
}
