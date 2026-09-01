package github

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/httperr"
)

// ReleaseExporter exports a repository's releases, their notes, and their
// uploaded assets from GitHub.com, using the same authenticated Client
// Lister and Remoter already use.
type ReleaseExporter struct {
	Client *Client
}

// NewReleaseExporter builds a ReleaseExporter against c.
func NewReleaseExporter(c *Client) *ReleaseExporter {
	return &ReleaseExporter{Client: c}
}

// Kind implements backup.MetadataExporter.
func (e *ReleaseExporter) Kind() backup.MetadataKind { return backup.MetadataReleases }

// ghRelease is one item from GET /repos/{owner}/{repo}/releases. Its Assets
// are only the files someone actually uploaded to the release -- GitHub
// serves the source archive it generates automatically for every tag
// through separate zipball_url/tarball_url fields this type doesn't even
// decode, so unlike ghIssue's PullRequest field there's nothing here to
// filter out.
type ghRelease struct {
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	Body        string     `json:"body"`
	Author      ghUser     `json:"author"`
	CreatedAt   time.Time  `json:"created_at"`
	PublishedAt *time.Time `json:"published_at"`
	Assets      []ghAsset  `json:"assets"`
}

// ghAsset is one item in a ghRelease's own "assets" array. URL, not
// BrowserDownloadURL, is what downloadAsset fetches from: it's the API
// endpoint, which returns the asset's raw content for an authenticated
// request that asks for it with an "Accept: application/octet-stream"
// header -- unlike browser_download_url, this also works for a private
// repository, the same reason Client.Remote itself never uses a forge's own
// reported clone URL where an authenticated API path is available instead.
type ghAsset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

// Export implements backup.MetadataExporter by paging through
// /repos/{owner}/{repo}/releases, downloading every uploaded asset on each
// one, and writing every release out with backup.WriteRelease -- including
// a release with no uploaded assets, source archives only.
func (e *ReleaseExporter) Export(ctx context.Context, repo backup.Repo, dir string) error {
	for page := 1; ; page++ {
		items, err := e.fetchReleasesPage(ctx, repo.Path, page)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}

		for _, it := range items {
			if err := e.exportRelease(ctx, repo.Path, dir, it); err != nil {
				return err
			}
		}

		if len(items) < pageSize {
			return nil
		}
	}
}

// exportRelease downloads every uploaded asset on it before writing
// release.json, so a partial failure downloading one asset never leaves a
// release.json on disk claiming an asset that isn't actually there.
func (e *ReleaseExporter) exportRelease(ctx context.Context, repoPath, dir string, it ghRelease) error {
	assets := make([]backup.ReleaseAsset, len(it.Assets))
	for i, a := range it.Assets {
		size, err := e.downloadAsset(ctx, repoPath, dir, it.TagName, a)
		if err != nil {
			return err
		}
		assets[i] = backup.ReleaseAsset{Name: a.Name, Size: size}
	}

	if err := backup.WriteRelease(dir, backup.Release{
		TagName: it.TagName, Name: it.Name, Body: it.Body, Author: it.Author.Login,
		CreatedAt: it.CreatedAt, PublishedAt: it.PublishedAt, Assets: assets,
	}); err != nil {
		return fmt.Errorf("write release %s#%s: %w", repoPath, it.TagName, err)
	}

	return nil
}

func (e *ReleaseExporter) downloadAsset(ctx context.Context, repoPath, dir, tagName string, a ghAsset) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return 0, fmt.Errorf("download github release asset %s for %s: %w", a.Name, repoPath, err)
	}
	req.Header.Set("Authorization", "Bearer "+e.Client.Token)
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)

	resp, err := e.Client.httpClient().Do(req)
	if err != nil {
		return 0, fmt.Errorf("download github release asset %s for %s: %w", a.Name, repoPath, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download github release asset %s for %s: %w: %d",
			a.Name, repoPath, httperr.ErrUnexpectedStatus, resp.StatusCode)
	}

	size, err := backup.WriteReleaseAsset(dir, tagName, a.Name, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("download github release asset %s for %s: %w", a.Name, repoPath, err)
	}

	return size, nil
}

func (e *ReleaseExporter) fetchReleasesPage(ctx context.Context, repoPath string, page int) ([]ghRelease, error) {
	u := e.Client.BaseURL.JoinPath("/repos/" + repoPath + "/releases")
	q := u.Query()
	q.Set("per_page", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	var items []ghRelease
	if err := getJSON(ctx, e.Client, u, &items); err != nil {
		return nil, fmt.Errorf("list github releases for %s: %w", repoPath, err)
	}

	return items, nil
}
