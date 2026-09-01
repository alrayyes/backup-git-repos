package forgejo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/httperr"
)

// ReleaseExporter exports a repository's releases, their notes, and their
// uploaded assets from a self-hosted Forgejo (or Gitea) instance, over the
// same REST API and the same authenticated Client Lister and Remoter
// already use.
type ReleaseExporter struct {
	Client *Client
}

// NewReleaseExporter builds a ReleaseExporter against c.
func NewReleaseExporter(c *Client) *ReleaseExporter {
	return &ReleaseExporter{Client: c}
}

// Kind implements backup.MetadataExporter.
func (e *ReleaseExporter) Kind() backup.MetadataKind { return backup.MetadataReleases }

// forgeRelease is one item from GET /repos/{owner}/{repo}/releases. Its
// Assets are only the files someone actually uploaded to the release --
// Forgejo serves the source archive it generates automatically for every
// tag through separate tarball_url/zipball_url fields this type doesn't
// even decode, so unlike ghIssue's PullRequest field there's nothing here
// to filter out.
type forgeRelease struct {
	TagName     string       `json:"tag_name"`
	Name        string       `json:"name"`
	Body        string       `json:"body"`
	Author      forgeUser    `json:"author"`
	CreatedAt   time.Time    `json:"created_at"`
	PublishedAt *time.Time   `json:"published_at"`
	Assets      []forgeAsset `json:"assets"`
}

// forgeAsset is one item in a forgeRelease's own "assets" array.
// BrowserDownloadURL is what downloadAsset fetches the content from,
// authenticated the same "token <t>" way every other request to this
// client is.
type forgeAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
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
func (e *ReleaseExporter) exportRelease(ctx context.Context, repoPath, dir string, it forgeRelease) error {
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

func (e *ReleaseExporter) downloadAsset(ctx context.Context, repoPath, dir, tagName string, a forgeAsset) (int64, error) {
	assetURL, err := e.resolveAssetURL(a.BrowserDownloadURL)
	if err != nil {
		return 0, fmt.Errorf("download forgejo release asset %s for %s: %w", a.Name, repoPath, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return 0, fmt.Errorf("download forgejo release asset %s for %s: %w", a.Name, repoPath, err)
	}
	req.Header.Set("Authorization", "token "+e.Client.Token)

	resp, err := e.Client.httpClient().Do(req)
	if err != nil {
		return 0, fmt.Errorf("download forgejo release asset %s for %s: %w", a.Name, repoPath, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download forgejo release asset %s for %s: %w: %d",
			a.Name, repoPath, httperr.ErrUnexpectedStatus, resp.StatusCode)
	}

	size, err := backup.WriteReleaseAsset(dir, tagName, a.Name, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("download forgejo release asset %s for %s: %w", a.Name, repoPath, err)
	}

	return size, nil
}

// resolveAssetURL rebuilds a Forgejo-reported browser_download_url against
// this client's own configured base URL, keeping only the path and query --
// the same reasoning Client.Remote already applies to a repository's own
// clone URL: Forgejo reports the host and port from its own ROOT_URL
// setting, which inside a container is the container's internal port, not
// whatever host and port the caller can actually reach it on.
func (e *ReleaseExporter) resolveAssetURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse asset url %s: %w", raw, err)
	}

	resolved := *e.Client.BaseURL
	resolved.Path = parsed.Path
	resolved.RawPath = parsed.RawPath
	resolved.RawQuery = parsed.RawQuery

	return resolved.String(), nil
}

func (e *ReleaseExporter) fetchReleasesPage(ctx context.Context, repoPath string, page int) ([]forgeRelease, error) {
	u := e.Client.BaseURL.JoinPath("/api/v1/repos/" + repoPath + "/releases")
	q := u.Query()
	q.Set("limit", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	var items []forgeRelease
	if err := e.Client.getJSON(ctx, u, &items); err != nil {
		return nil, fmt.Errorf("list forgejo releases for %s: %w", repoPath, err)
	}

	return items, nil
}
