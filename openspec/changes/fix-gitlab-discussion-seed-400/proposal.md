## Why

Every `internal/gitlab` test that calls `start(t)` fails deterministically
against a real GitLab CE 19.2.2 container: `fixture.seed`'s
`seedOpenMergeRequestWithReviewComment` gets a 400 posting a diff-anchored
discussion right after creating the merge request. `git blame` traces the
code to #116 (pull/merge request metadata export), merged 2026-09-01 --
this is the first nightly `gitlab`-tagged run since. Tracked as GitHub
issue #123, found while auditing #102.

## What changes

- Diagnose why GitLab CE 19.2.2 rejects the discussion POST that used to
  work: likely the merge request's own `diff_refs`
  (`base_sha`/`start_sha`/`head_sha`), read immediately after creation in
  `internal/gitlab/fixture_test.go`'s `seedOpenMergeRequestWithReviewComment`,
  aren't yet consistent server-side.
- Fix the fixture (a fresh `GET` of the merge request before posting the
  discussion, a short retry, or whatever the real cause turns out to need),
  with a comment at the call site explaining why.
- No production code changes expected -- this is test fixture seeding
  against a real container, not the library's own discussion-posting path.

## Capabilities

### New capabilities

None.

### Modified capabilities

None -- test-fixture-only, no requirement changes. `skip_specs: true` set
in this change's `.openspec.yaml`.

## Impact

- `internal/gitlab/fixture_test.go`.
- Every `//go:build integration && gitlab` test in `internal/gitlab` that
  calls `start(t)` is currently blocked by this and must pass afterward.
