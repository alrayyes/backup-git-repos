package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/httperr"
)

// uploadsPathPattern matches the trailing "/uploads/<secret>/<filename>" GitLab appends to a
// project's own path for a file uploaded through POST .../uploads -- the same route markdown
// image attachments use, and the shape a release link built from one comes back as, whether in
// its own "url" field or in "direct_asset_url". That route is a web page gated by a browser
// session, not PRIVATE-TOKEN -- see the comment on resolveAssetURL for how that's worked
// around.
var uploadsPathPattern = regexp.MustCompile(`/uploads/[^/]+/[^/]+$`)

// ReleaseExporter exports a project's releases, their notes, and their
// uploaded assets from a self-hosted GitLab instance, using the same
// authenticated Client Lister and Remoter already use.
type ReleaseExporter struct {
	Client *Client
}

// NewReleaseExporter builds a ReleaseExporter against c.
func NewReleaseExporter(c *Client) *ReleaseExporter {
	return &ReleaseExporter{Client: c}
}

// Kind implements backup.MetadataExporter.
func (e *ReleaseExporter) Kind() backup.MetadataKind { return backup.MetadataReleases }

// glRelease is one item from GET /api/v4/projects/:id/releases. Its
// Assets.Links are only the ones someone actually attached to the release --
// GitLab reports the source archive it generates automatically for every
// tag under the separate Assets.Sources, which this type doesn't even
// decode, so unlike ghIssue's PullRequest field there's nothing here to
// filter out.
type glRelease struct {
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Author      glUser     `json:"author"`
	CreatedAt   time.Time  `json:"created_at"`
	ReleasedAt  *time.Time `json:"released_at"`
	Assets      glAssets   `json:"assets"`
}

type glAssets struct {
	Links []glAssetLink `json:"links"`
}

// glAssetLink is one item in a glRelease's own "assets.links" array.
// DirectAssetURL is what downloadAsset fetches from when it's set: GitLab's
// own docs describe it as the permanent redirect to an asset's actual
// location, which stays authenticatable with this client's own token even
// for a link GitLab generated for its own generic package registry --
// unlike URL, which a release link can carry pointing anywhere, including a
// host this token means nothing to.
type glAssetLink struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	DirectAssetURL string `json:"direct_asset_url"`
}

// Export implements backup.MetadataExporter by paging through
// /api/v4/projects/:id/releases, downloading every uploaded asset link on
// each one, and writing every release out with backup.WriteRelease --
// including a release with no uploaded assets, source archives only.
func (e *ReleaseExporter) Export(ctx context.Context, repo backup.Repo, dir string) error {
	for page := 1; ; page++ {
		items, next, err := e.fetchReleasesPage(ctx, repo.Path, page)
		if err != nil {
			return err
		}

		for _, it := range items {
			if err := e.exportRelease(ctx, repo.Path, dir, it); err != nil {
				return err
			}
		}

		if next == "" {
			return nil
		}
	}
}

// exportRelease downloads every uploaded asset link on it before writing
// release.json, so a partial failure downloading one asset never leaves a
// release.json on disk claiming an asset that isn't actually there.
func (e *ReleaseExporter) exportRelease(ctx context.Context, projectPath, dir string, it glRelease) error {
	assets := make([]backup.ReleaseAsset, len(it.Assets.Links))
	for i, l := range it.Assets.Links {
		size, err := e.downloadAsset(ctx, projectPath, dir, it.TagName, l)
		if err != nil {
			return err
		}
		assets[i] = backup.ReleaseAsset{Name: l.Name, Size: size}
	}

	if err := backup.WriteRelease(dir, backup.Release{
		TagName: it.TagName, Name: it.Name, Body: it.Description, Author: it.Author.Username,
		CreatedAt: it.CreatedAt, PublishedAt: it.ReleasedAt, Assets: assets,
	}); err != nil {
		return fmt.Errorf("write release %s#%s: %w", projectPath, it.TagName, err)
	}

	return nil
}

func (e *ReleaseExporter) downloadAsset(ctx context.Context, projectPath, dir, tagName string, l glAssetLink) (int64, error) {
	assetURL, err := e.resolveAssetURL(projectPath, l)
	if err != nil {
		return 0, fmt.Errorf("download gitlab release asset %s for %s: %w", l.Name, projectPath, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return 0, fmt.Errorf("download gitlab release asset %s for %s: %w", l.Name, projectPath, err)
	}
	req.Header.Set("PRIVATE-TOKEN", e.Client.Token)

	resp, err := e.Client.httpClient().Do(req)
	if err != nil {
		return 0, fmt.Errorf("download gitlab release asset %s for %s: %w", l.Name, projectPath, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download gitlab release asset %s for %s: %w: %d",
			l.Name, projectPath, httperr.ErrUnexpectedStatus, resp.StatusCode)
	}

	// GitLab's uploads-based download URLs (as opposed to a direct external
	// link) are a web route authenticated by browser session, not the
	// PRIVATE-TOKEN header this client sends -- an unauthenticated request
	// gets silently redirected to the sign-in page, which itself answers 200
	// OK, so the status check above never catches it. The sign-in page is
	// the only thing on this path that's ever HTML; a real asset never is.
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
		return 0, fmt.Errorf("download gitlab release asset %s for %s: %w: got html, likely the sign-in page",
			l.Name, projectPath, httperr.ErrUnauthenticatedRedirect)
	}

	size, err := backup.WriteReleaseAsset(dir, tagName, l.Name, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("download gitlab release asset %s for %s: %w", l.Name, projectPath, err)
	}

	return size, nil
}

// resolveAssetURL returns the URL downloadAsset should fetch l's content
// from. DirectAssetURL, when GitLab set it, is always a link on this same
// GitLab instance -- generated from GitLab's own configured external URL,
// which inside a container is the container's internal port, not whatever
// host and port the caller can actually reach it on, the same reasoning
// Client.Remote already applies to a repository's own clone URL -- so it's
// rebuilt against this client's own configured base URL, keeping only the
// path and query. URL, GitLab's fallback when no direct_asset_url is set,
// can point anywhere -- an external link someone manually attached to the
// release -- so it's used as-is rather than assumed to live on this
// instance.
//
// Either one can come back matching uploadsPathPattern: a release asset
// someone attached by uploading a file directly (rather than through the
// generic package registry) resolves to that same web-only path in both
// fields, confirmed live -- GitLab never computes a distinct
// direct_asset_url for an upload-backed link the way it does for a
// package-registry one. GitLab's own API docs (project_markdown_uploads)
// describe a second route serving the identical upload that does accept
// PRIVATE-TOKEN -- GET /api/v4/projects/:id/uploads/:secret/:filename -- so
// a matching path is rewritten onto that route instead of fetched as the
// literal URL GitLab handed back.
func (e *ReleaseExporter) resolveAssetURL(projectPath string, l glAssetLink) (string, error) {
	candidate := l.URL
	if l.DirectAssetURL != "" {
		candidate = l.DirectAssetURL
	}

	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", fmt.Errorf("parse asset url %s: %w", candidate, err)
	}

	if uploadsPath := uploadsPathPattern.FindString(parsed.Path); uploadsPath != "" {
		return e.Client.BaseURL.JoinPath("/api/v4/projects/" + url.PathEscape(projectPath) + uploadsPath).String(), nil
	}

	if l.DirectAssetURL == "" {
		return l.URL, nil
	}

	resolved := *e.Client.BaseURL
	resolved.Path = parsed.Path
	resolved.RawPath = parsed.RawPath
	resolved.RawQuery = parsed.RawQuery

	return resolved.String(), nil
}

// fetchReleasesPage returns one page of projectPath's releases and the
// value of the x-next-page response header, empty on the last page. A 403
// -- the same "feature off or token lacks permission" ambiguity
// getOptional's own doc comment already describes for wikis and snippets --
// is treated as no releases rather than an error.
func (e *ReleaseExporter) fetchReleasesPage(ctx context.Context, projectPath string, page int) ([]glRelease, string, error) {
	u := e.Client.projectSubURL(projectPath, "releases", page)

	var items []glRelease
	next, err := e.Client.getOptional(ctx, u, "releases", projectPath, &items)
	if err != nil {
		return nil, "", fmt.Errorf("list gitlab releases for %s: %w", projectPath, err)
	}

	return items, next, nil
}
