## Why

A bare mirror captures commits, branches and tags, but a repository's pull
and merge requests -- and the review discussion attached to them -- live
outside the git object graph entirely. If the forge hosting a repository
goes away permanently, that review history is lost even though the code
survives. #81 already exports issues this way and #83 does the same for
releases; pull/merge requests are the third kind split out of the original
umbrella ticket (#7).

## What changes

- Add a `MetadataPullRequests` `MetadataKind` (`"pull-requests"`) to the set
  `--export-metadata` accepts, alongside the existing `issues` and
  `releases`.
- Add a `PullRequestExporter` `MetadataExporter` per forge adapter
  (`internal/forgejo`, `internal/github`, `internal/gitlab`), following the
  same shape `IssueExporter` and `ReleaseExporter` already establish: a
  constructor taking the forge's client, `Kind()`, and `Export(ctx, repo,
dir) error`.
- Wire each new exporter into that forge's `Runner` in
  `cmd/backup-git-repos/main.go`'s `newRunner`, alongside the existing
  issue and release exporters.
- Write one JSON file per pull/merge request under the same
  `<dest>/<repo.Path>.metadata/<kind>/` convention `metadata.go` already
  documents, including title, body, author, state, source/target branch,
  timestamps, and its review comments -- both general (issue-style) and
  inline, diff-anchored ones, with the file along with the line each
  inline comment anchors to preserved.
- Document the new `--export-metadata pull-requests` value, its on-disk
  format, and what's excluded, in the README's existing Metadata export
  section.

## Capabilities

### New capabilities

- `metadata-export/pull-requests`: exporting a repository's pull and merge
  requests, including general and inline review comments, to the
  `<dest>/<repo.Path>.metadata/pull-requests/` directory when
  `--export-metadata` selects it.

### Modified capabilities

(none -- this adds a new metadata kind without changing how the existing
`issues` or `releases` kinds behave)

## Impact

- `metadata.go`: new `MetadataPullRequests` constant, added to
  `knownMetadataKinds`; new `PullRequest`/`PullRequestComment` types and a
  `WritePullRequest` helper alongside the existing `Issue`/`Release`
  writers.
- `internal/forgejo`, `internal/github`, `internal/gitlab`: one new
  `PullRequestExporter` each. Its fixtures and tests follow the existing
  issue/release exporter test patterns.
- `cmd/backup-git-repos/main.go`: `newRunner` wires the new exporter into
  each forge's `Runner.MetadataExporters`.
- `cli.go`: the `--export-metadata` flag's help text gains `pull-requests`.
- `README.md`: metadata export section documents the new kind, its format,
  and what's excluded (for example, review approvals/status checks, if
  scoped out -- see design.md).
- No change to existing `issues` or `releases` export behavior, and no
  change at all when `--export-metadata` doesn't select `pull-requests`.
