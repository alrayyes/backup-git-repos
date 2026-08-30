package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// MetadataKind identifies one kind of forge metadata a MetadataExporter can
// write alongside a repository's mirror -- metadata a bare git mirror never
// captures, because none of it lives in the git object graph. MetadataIssues
// is the only kind this package defines; #82, #83 and #84 (pull/merge
// requests, releases, and CI/CD config, all split out of the same parent
// ticket as issues) each add their own constant here and their own
// MetadataExporter per forge adapter, without changing this type, the
// MetadataExporter interface below, or the on-disk layout it documents.
type MetadataKind string

// MetadataIssues selects a repository's issues and their comments.
const MetadataIssues MetadataKind = "issues"

// knownMetadataKinds are the values --export-metadata accepts.
var knownMetadataKinds = map[MetadataKind]bool{
	MetadataIssues: true,
}

// ErrBadMetadataKind means --export-metadata named a kind this build
// doesn't know how to export.
var ErrBadMetadataKind = errors.New("export-metadata must be one of: issues")

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
// supports a kind implements one MetadataExporter per kind -- this PR adds
// an issues MetadataExporter to internal/gitlab, internal/forgejo and
// internal/github, following the same shape as those packages' existing
// Lister and Remoter. A future MetadataKind (pull/merge requests, releases,
// CI/CD config) gets its own MetadataExporter type per adapter package,
// wired into that forge's Runner in cmd/backup-git-repos/main.go's newRunner
// alongside the issues one -- no change needed here.
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
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
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
