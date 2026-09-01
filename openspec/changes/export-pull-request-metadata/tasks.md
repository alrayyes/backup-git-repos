## Shared types and writer

- [ ] 1.1 Add `MetadataPullRequests MetadataKind = "pull-requests"` to
      `metadata.go` and register it in `knownMetadataKinds`; verify
      `ParseMetadataKinds([]string{"pull-requests"})` returns it and an
      unknown kind still errors
- [ ] 1.2 Add `PullRequest` and `PullRequestComment` types to `metadata.go`
      per design.md's Decisions section; verify they marshal to the
      documented JSON shape via a table test in `metadata_test.go`
- [ ] 1.3 Add `WritePullRequest(dir string, pr PullRequest) error`,
      following `WriteIssue`'s shape (nil `Comments` normalized to `[]`,
      writes `<dir>/<pr.Number>.json`); verify with a unit test covering a
      PR with comments, one with none, and one that's merged vs. closed
      vs. open

## GitHub exporter

- [ ] 2.1 Add `internal/github/release.go`-style `PullRequestExporter`
      (`NewPullRequestExporter`, `Kind()`, `Export()`) fetching
      `GET /repos/{owner}/{repo}/pulls?state=all`, paged; verify against a
      recorded/fixture test following `internal/github/release_test.go`'s
      pattern
- [ ] 2.2 Fetch general comments via `/issues/{number}/comments` and
      inline comments via `/pulls/{number}/comments`, merging both into
      one `PullRequest.Comments`, paged for each; verify a fixture PR with
      both comment kinds round-trips correctly, and comment ordering is by
      creation time
- [ ] 2.3 Wire `github.NewPullRequestExporter` into
      `cmd/backup-git-repos/main.go`'s `newRunner` for the `"github"` case

## Forgejo exporter

- [ ] 3.1 Add `internal/forgejo/pull_request.go` `PullRequestExporter`
      fetching `GET /repos/{owner}/{repo}/pulls?state=all`, paged; verify
      against a recorded/fixture test following
      `internal/forgejo/release_test.go`'s pattern
- [ ] 3.2 Fetch general comments via `/issues/{index}/comments` and inline
      comments via `/pulls/{index}/reviews` (flattening each review's own
      comments into `PullRequest.Comments`); verify a fixture PR with a
      review containing multiple inline comments round-trips correctly
- [ ] 3.3 Wire `forgejo.NewPullRequestExporter` into
      `cmd/backup-git-repos/main.go`'s `newRunner` for the `"forgejo"` case

## GitLab exporter

- [ ] 4.1 Add `internal/gitlab/merge_request.go` `PullRequestExporter`
      fetching `GET /api/v4/projects/:id/merge_requests?state=all`, paged;
      verify against a recorded/fixture test following
      `internal/gitlab/release_test.go`'s pattern
- [ ] 4.2 Fetch comments via `/merge_requests/:iid/discussions`, filtering
      system notes the way `IssueExporter.fetchNotes` already does, and
      mapping `position.new_path`/`position.new_line` onto
      `PullRequestComment.Path`/`Line` when present; verify a fixture MR
      with both a general note and a positioned discussion note round-trips
      correctly
- [ ] 4.3 Wire `gitlab.NewPullRequestExporter` into
      `cmd/backup-git-repos/main.go`'s `newRunner` for the `"gitlab"` case

## CLI and docs

- [ ] 5.1 Update `cli.go`'s `--export-metadata` help text to list
      `pull-requests` alongside `issues` and `releases`
- [ ] 5.2 Document `--export-metadata pull-requests` in README's Metadata
      export section: the on-disk format
      (`<dest>/<repo>.metadata/pull-requests/<number>.json`), an example
      file tree entry alongside the existing issues/releases examples, and
      what's excluded (approvals, status checks, surrounding diff context
      -- per design.md's Non-Goals)

## Verification

- [ ] 6.1 Run `go test ./...` and the `-tags=integration` lane; verify both
      pass with no regressions to the existing `issues`/`releases` exporters
- [ ] 6.2 Confirm `--export-metadata` with `pull-requests` omitted leaves
      behavior byte-for-byte unchanged from before this change (no new
      directory, no new API calls) via the existing metadata-export tests
      in `metadata_test.go`
