package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// MetadataKind identifies one kind of forge metadata a MetadataExporter can
// write alongside a repository's mirror -- metadata a bare git mirror never
// captures, because none of it lives in the git object graph. MetadataIssues
// and MetadataReleases are the kinds this package defines so far; #82 and
// #84 (pull/merge requests and CI/CD config, split out of the same parent
// ticket as issues and releases) each add their own constant here and their
// own MetadataExporter per forge adapter, without changing this type, the
// MetadataExporter interface below, or the on-disk layout it documents.
type MetadataKind string

// MetadataIssues selects a repository's issues and their comments.
const MetadataIssues MetadataKind = "issues"

// MetadataReleases selects a repository's releases: their notes and their
// uploaded assets, not the source archives a forge generates automatically
// for every tag.
const MetadataReleases MetadataKind = "releases"

// knownMetadataKinds are the values --export-metadata accepts.
var knownMetadataKinds = map[MetadataKind]bool{
	MetadataIssues:   true,
	MetadataReleases: true,
}

// ErrBadMetadataKind means --export-metadata named a kind this build
// doesn't know how to export.
var ErrBadMetadataKind = errors.New("export-metadata must be one of: issues, releases")

// ParseMetadataKinds parses --export-metadata's repeatable,
// comma-separated values into the set of MetadataKind a Run should export.
// A nil or empty vals returns a nil, empty result -- metadata export
// disabled, which is what leaves a Run's behavior unchanged from before
// this option existed (see MetadataExporter's own doc comment, and #81's
// acceptance criteria).
func ParseMetadataKinds(vals []string) ([]MetadataKind, error) {
	if len(vals) == 0 {
		return nil, nil
	}

	kinds := make([]MetadataKind, 0, len(vals))
	for _, v := range vals {
		k := MetadataKind(v)
		if !knownMetadataKinds[k] {
			return nil, fmt.Errorf("%w: got %q", ErrBadMetadataKind, v)
		}
		kinds = append(kinds, k)
	}

	return kinds, nil
}

// MetadataExporter writes one MetadataKind of a repository's forge metadata
// into a directory Run has already created for it. Each forge adapter that
// supports a kind implements one MetadataExporter per kind -- #81 added an
// issues MetadataExporter to internal/gitlab, internal/forgejo and
// internal/github, and this one adds a releases MetadataExporter alongside
// it, following the same shape as those packages' existing Lister and
// Remoter. A future MetadataKind (pull/merge requests, CI/CD config) gets
// its own MetadataExporter type per adapter package, wired into that
// forge's Runner in cmd/backup-git-repos/main.go's newRunner alongside the
// existing ones -- no change needed here.
//
// On-disk layout: Run writes a kind's output under
// "<dest>/<repo.Path>.metadata/<kind>/" -- a sibling of the repository's own
// bare mirror ("<dest>/<repo.Path>.git") and, when --archive selects it, its
// tar.gz ("<archive-dir>/<repo.Path>.tar.gz"). None of the three collide,
// and a later MetadataKind gets its own sibling subdirectory under the same
// ".metadata" parent -- "<dest>/<repo.Path>.metadata/pull-requests/", say --
// rather than a naming convention of its own to invent.
type MetadataExporter interface {
	// Kind reports which MetadataKind this exporter handles, so Run can
	// match it against Options.ExportMetadata without a type switch.
	Kind() MetadataKind

	// Export writes repo's metadata of this Kind into dir, which already
	// exists by the time Export is called.
	Export(ctx context.Context, repo Repo, dir string) error
}

// wantsMetadata reports whether kind is one of the kinds opts.ExportMetadata
// asked for.
func wantsMetadata(kinds []MetadataKind, kind MetadataKind) bool {
	return slices.Contains(kinds, kind)
}

// metadataDir is where a repository's exported metadata lives on disk, a
// sibling of its own mirrorPath and archivePath -- see MetadataExporter's
// own doc comment for the full convention.
func metadataDir(dest, path string) string {
	return filepath.Join(dest, filepath.FromSlash(path)+".metadata")
}

// Issue is one forge issue and its full comment thread -- the format every
// MetadataIssues exporter writes, one JSON file per issue, named by its
// number, under "<dest>/<repo.Path>.metadata/issues/<number>.json". Every
// forge (GitLab, Forgejo, GitHub) maps its own issue shape onto this one, so
// a file this tool wrote reads the same regardless of which forge it came
// from. ClosedAt is nil for an issue still open. Comments is always a
// (possibly empty, never nil) slice: an issue with no comments is still
// written with "comments": [] rather than "comments": null or being
// skipped entirely -- #81's own acceptance criteria.
type Issue struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Author    string     `json:"author"`
	State     string     `json:"state"` // "open" or "closed"
	Labels    []string   `json:"labels"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
	Comments  []Comment  `json:"comments"`
}

// Comment is one comment on an Issue.
type Comment struct {
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// WriteIssue marshals issue as indented JSON and writes it to
// "<dir>/<issue.Number>.json". dir must already exist -- Run creates it once
// before calling an exporter's Export at all (see MetadataExporter's own
// doc comment), rather than every WriteIssue call re-creating a directory
// that's already there, which is wasted work once a repository's issue
// count gets large. Shared by every forge's issues MetadataExporter so the
// file layout and JSON formatting -- two spaces, one file per issue, a nil
// Comments normalized to an empty slice rather than encoded as null --
// stays identical regardless of which forge wrote it.
func WriteIssue(dir string, issue Issue) error {
	if issue.Comments == nil {
		issue.Comments = []Comment{}
	}

	data, err := json.MarshalIndent(issue, "", "  ")
	if err != nil {
		return fmt.Errorf("write issue %d: %w", issue.Number, err)
	}

	path := filepath.Join(dir, strconv.Itoa(issue.Number)+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write issue %d: %w", issue.Number, err)
	}

	return nil
}

// Release is one forge release, its notes, and the uploaded assets attached
// to it -- the format every MetadataReleases exporter writes, one directory
// per release named by its (sanitized) tag, under
// "<dest>/<repo.Path>.metadata/releases/<tag>/release.json". PublishedAt is
// nil for a release that's still a draft. Assets is always a (possibly
// empty, never nil) slice: a release with no uploaded assets -- source
// archives only -- is still written with "assets": [] and, unlike
// WriteIssue's own comments, with no "assets" directory created at all --
// #83's own acceptance criteria is "no empty asset files", and an exporter
// that never calls WriteReleaseAsset for a release never creates that
// directory in the first place, so there's nothing here to special-case.
type Release struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	Author      string         `json:"author"`
	CreatedAt   time.Time      `json:"created_at"`
	PublishedAt *time.Time     `json:"published_at,omitempty"`
	Assets      []ReleaseAsset `json:"assets"`
}

// ReleaseAsset describes one uploaded asset on a Release. Its actual
// content lives on disk at "<dir>/<release's tag>/assets/<Name>", written
// by WriteReleaseAsset -- not encoded into release.json itself, since an
// asset can be arbitrarily large and JSON has no way to hold binary content
// without inflating it by a third through base64.
type ReleaseAsset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// MaxReleaseAssetSize bounds how much of a single release asset
// WriteReleaseAsset will write to disk before giving up. A release asset
// can be arbitrarily large -- a multi-gigabyte installer or archive isn't
// unusual -- and streaming one with no limit at all risks a single runaway
// asset ballooning disk usage well past what a caller expected from one
// repository's backup. 4 GiB comfortably covers an ordinary binary,
// installer, or container image tarball while still catching a response
// that's gone wrong. The per-repository --timeout already bounds how long
// any one asset (or release, or the whole export) is allowed to take; this
// bounds how much it's allowed to write in that time.
const MaxReleaseAssetSize = 4 << 30 // 4 GiB

// ErrReleaseAssetTooLarge means a release asset's content exceeded
// MaxReleaseAssetSize -- WriteReleaseAsset stops writing it and removes the
// partial file rather than silently leaving a truncated one behind that
// would read as complete.
var ErrReleaseAssetTooLarge = errors.New("release asset exceeds the maximum size backup-git-repos will export")

// releaseDir is where one release's release.json and assets/ live, a
// sibling of every other release's own directory under the same
// MetadataReleases dir Run created -- see Release's own doc comment for the
// full convention.
func releaseDir(dir, tagName string) string {
	return filepath.Join(dir, sanitizeName(tagName))
}

// sanitizeName maps a forge-supplied name -- a release tag or an asset's
// own filename, both under whoever created the release's control rather
// than this tool's -- to a single, safe path segment. "/" (a git tag can
// legitimately contain one, "releases/v1" is a valid tag) and "\" are
// replaced rather than allowed to add directory nesting nobody asked for,
// and "", ".", ".." -- which would otherwise resolve to dir itself or
// escape it entirely -- collapse to a literal placeholder instead.
func sanitizeName(name string) string {
	name = strings.NewReplacer("/", "-", "\\", "-").Replace(name)
	if name == "" || name == "." || name == ".." {
		return "_"
	}

	return name
}

// WriteRelease marshals release as indented JSON and writes it to
// "<dir>/<release.TagName sanitized>/release.json". dir must already exist
// -- Run creates it once before calling an exporter's Export at all (see
// MetadataExporter's own doc comment). Shared by every forge's releases
// MetadataExporter so the file layout and JSON formatting stay identical
// regardless of which forge wrote it, the same reasoning WriteIssue already
// follows for issues.
func WriteRelease(dir string, release Release) error {
	if release.Assets == nil {
		release.Assets = []ReleaseAsset{}
	}

	data, err := json.MarshalIndent(release, "", "  ")
	if err != nil {
		return fmt.Errorf("write release %s: %w", release.TagName, err)
	}

	rd := releaseDir(dir, release.TagName)
	if err := os.MkdirAll(rd, 0o750); err != nil {
		return fmt.Errorf("write release %s: %w", release.TagName, err)
	}

	if err := os.WriteFile(filepath.Join(rd, "release.json"), data, 0o600); err != nil {
		return fmt.Errorf("write release %s: %w", release.TagName, err)
	}

	return nil
}

// WriteReleaseAsset streams body -- typically an HTTP response body an
// exporter is still reading from, never buffered into memory first -- into
// "<dir>/<tagName sanitized>/assets/<assetName sanitized>", creating that
// assets directory only once a release actually has an asset to put in it.
// It returns the number of bytes actually written, which an exporter uses
// as ReleaseAsset.Size rather than trusting a forge's own reported size --
// GitLab's release links carry no size field at all, and even where one
// does, the byte count of what's actually on disk is the more honest value
// to record. Capped at MaxReleaseAssetSize: a response that keeps producing
// bytes past the cap fails with ErrReleaseAssetTooLarge and the partial
// file is removed, rather than silently writing a truncated asset that
// would read as complete.
func WriteReleaseAsset(dir, tagName, assetName string, body io.Reader) (int64, error) {
	assetDir := filepath.Join(releaseDir(dir, tagName), "assets")
	if err := os.MkdirAll(assetDir, 0o750); err != nil {
		return 0, fmt.Errorf("write release asset %s: %w", assetName, err)
	}

	path := filepath.Join(assetDir, sanitizeName(assetName))

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // path is built from releaseDir/sanitizeName, not raw untrusted input
	if err != nil {
		return 0, fmt.Errorf("write release asset %s: %w", assetName, err)
	}

	n, copyErr := io.Copy(f, io.LimitReader(body, MaxReleaseAssetSize+1))
	closeErr := f.Close()

	switch {
	case copyErr != nil:
		_ = os.Remove(path)

		return 0, fmt.Errorf("write release asset %s: %w", assetName, copyErr)
	case n > MaxReleaseAssetSize:
		_ = os.Remove(path)

		return 0, fmt.Errorf("write release asset %s: %w", assetName, ErrReleaseAssetTooLarge)
	case closeErr != nil:
		return 0, fmt.Errorf("write release asset %s: %w", assetName, closeErr)
	default:
		return n, nil
	}
}
