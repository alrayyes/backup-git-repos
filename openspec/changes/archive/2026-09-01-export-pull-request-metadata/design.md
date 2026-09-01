## Context

`metadata.go` already defines the pattern this change extends: a
`MetadataKind` constant, a `MetadataExporter` interface each forge adapter
implements once per kind, and one JSON file per item under
`<dest>/<repo.Path>.metadata/<kind>/`. Two kinds exist today:

- `issues` (`internal/*/issue.go`): one `IssueExporter` per forge, sharing
  `backup.Issue`/`backup.Comment` and `backup.WriteIssue`.
- `releases` (`internal/*/release.go`): one `ReleaseExporter` per forge,
  sharing `backup.Release`/`backup.ReleaseAsset` and `backup.WriteRelease`.

Each forge's issues endpoint already has a quirk this change inherits:
GitHub's (and Forgejo's, which mirrors GitHub's API shape) `GET
/repos/{owner}/{repo}/issues` returns pull requests alongside real issues,
distinguished only by a `pull_request` field the existing `IssueExporter`s
already use to skip them (`internal/github/issue.go`, `ghIssue.PullRequest`).
GitLab keeps merge requests on a wholly separate `/merge_requests` endpoint
from the start (`internal/gitlab/issue.go`'s own doc comment already notes
this for issues). See proposal.md for the ticket and motivation (#82).

## Goals / non-goals

**Goals:**

- Export every pull/merge request (open and closed/merged) with title,
  body, author, state, source/target branch, and timestamps.
- Export both general (issue-style) comments and inline, diff-anchored
  review comments, preserving the file along with the line an inline
  comment anchors to.
- Follow the existing `MetadataExporter` shape exactly -- no new interface,
  no new on-disk convention beyond a new `<kind>` directory name.

**Non-Goals:**

- Review _approvals_/requested-changes state, CI status checks, and commit
  lists on the PR/MR are not exported. They're either derivable from the
  mirrored git history (commits) or not meaningfully "review discussion"
  (approval state is a snapshot, not a preservable artifact the way a
  comment's text is). Worth a follow-up ticket if wanted later, not part
  of this change.
- Diff _content_ around an inline comment (a few lines of surrounding
  context) is not captured -- only the file/line reference. The commit
  history in the mirror already has the diff; duplicating it here would
  make this exporter re-implement `git diff`.
- No change to how `issues` already skips pull requests, or how `releases`
  behaves. This only adds a third, independent kind.

## Decisions

**Shared types live in `metadata.go`, next to `Issue`/`Release`.**
A `PullRequest` struct (`Number`, `Title`, `Body`, `Author`, `State`,
`SourceBranch`, `TargetBranch`, `CreatedAt`, `UpdatedAt`, `ClosedAt
*time.Time`, `MergedAt *time.Time`, `Comments []PullRequestComment`) and a
`PullRequestComment` struct that extends the existing `Comment` shape
(`Author`, `Body`, `CreatedAt`) with two optional fields, `Path` and `Line
*int` -- present for an inline comment, both zero/nil for a general one.
One comment type rather than two (`GeneralComment` vs `InlineComment`)
keeps `Comments` a single slice orderable by time, and keeps the JSON shape
close to `Issue.Comments`, which every forge's general PR comments already
map onto directly.

_Alternative considered_: a separate `ReviewComment` type and a second
`ReviewComments` field on `PullRequest`. Rejected -- it forces every
consumer of the JSON to merge and re-sort two lists to get a single
chronological thread, which is the more common thing to want.

**File layout**: `WritePullRequest(dir string, pr PullRequest) error`
writes `<dir>/<pr.Number>.json`, mirroring `WriteIssue` exactly (pull
requests are numbered the same way issues are on GitHub/Forgejo, and MR
IIDs the same way on GitLab -- there's no tag-like string identity here the
way a release has, so no `sanitizeName` step is needed).

**`MetadataPullRequests MetadataKind = "pull-requests"`**, added to
`knownMetadataKinds`, following the plural-noun naming `issues` and
`releases` already use.

**Per-forge fetch strategy**:

- _GitHub_ (`internal/github`): list via `GET
/repos/{owner}/{repo}/pulls?state=all`; general comments via `GET
/repos/{owner}/{repo}/issues/{number}/comments` (same endpoint
  `IssueExporter` already calls, since GitHub treats every PR as an issue);
  inline comments via `GET /repos/{owner}/{repo}/pulls/{number}/comments`,
  which carries `path` and `line`.
- _Forgejo_ (`internal/forgejo`): list via `GET
/repos/{owner}/{repo}/pulls?state=all`; general comments via the same
  `/issues/{index}/comments` endpoint `IssueExporter` uses; inline comments
  via `GET /repos/{owner}/{repo}/pulls/{index}/reviews` (Forgejo nests
  inline comments under a review, unlike GitHub's flat list -- the exporter
  flattens them into the same `PullRequestComment` shape).
- _GitLab_ (`internal/gitlab`): list via `GET
/api/v4/projects/:id/merge_requests?state=all`; all comments (general and
  inline) via `GET /api/v4/projects/:id/merge_requests/:iid/discussions`,
  filtering out system notes the same way `IssueExporter.fetchNotes`
  already does, and reading `position.new_path`/`position.new_line` when
  present for an inline note.

**No new shared helper across forges for the fetch itself** -- the three
APIs differ enough (nested reviews vs. flat comments vs. discussions) that
a shared abstraction would just be an indirection over three different
shapes, the same reasoning that already keeps `IssueExporter` and
`ReleaseExporter` forge-specific today. The only shared code is the
`PullRequest`/`PullRequestComment` types and `WritePullRequest`.

## Risks / trade-offs

- **Comment volume**: a long-lived, heavily reviewed PR can have hundreds
  of inline comments across many pages → mitigated the same way issue
  comments already are: page through every result rather than assuming one
  page (`internal/github/issue.go`'s `fetchComments` is the existing
  pattern to follow).
- **Forgejo's nested review structure** adds one extra fetch per PR
  (reviews, then each review's comments) compared to GitHub's flat list →
  accepted; Forgejo's own API gives no flatter alternative.
- **GitLab discussions mixing general and inline notes in one endpoint**
  means the exporter must branch on `position` presence per note rather
  than calling two different endpoints → accepted, matches GitLab's own
  API shape rather than fighting it.
