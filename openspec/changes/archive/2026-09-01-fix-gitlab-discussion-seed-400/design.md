## Context

See proposal.md. Confirmed via the nightly lane's own log: every
container-booting test that reaches `fixture.seed` fails at the exact same
call, deterministically (not intermittently), each time with `"400" is not
less than "300"` on `POST
/api/v4/projects/team%2Factive-repo/merge_requests/1/discussions`.
`internal/gitlab/fixture_test.go` carries zero diff on #102's branch, so
this isn't a concurrency artifact.

## Goals / non-goals

**Goals:**

- `go test -tags='integration gitlab' -race -timeout=35m ./internal/gitlab/...`
  passes end to end against a real GitLab CE 19.2.2 container.

**Non-goals:**

- Changing the library's own discussion/review-comment posting logic
  (`internal/gitlab/pull_request.go` or similar) unless the real cause
  turns out to live there rather than in the fixture.

## Decisions

- **Poll for real readiness rather than a fixed sleep.** The response body,
  surfaced live, was `{"message":"400 Bad request - Note
{:line_code=>[\"can't be blank\", \"must be a valid line code\"],
:position=>[\"is incomplete\"], :noteable=>[\"doesn't support new-style
diff notes\"]}"}`. A diagnostic `GET .../merge_requests/:iid/diffs`
  called right after creation confirmed the actual cause: `diff_refs` comes
  back completely empty (`{BaseSHA: StartSHA: HeadSHA:}`) and the diffs
  list is empty too -- GitLab computes a merge request's diff
  asynchronously, and the create response's own `diff_refs` can't be
  trusted. `waitForMergeRequestDiff` polls `GET .../merge_requests/:iid`
  every 500 ms (30 s timeout) until `diff_refs.head_sha` is non-empty, then
  hands back the fresh values for the discussion's position.
- **The merge step needed the same treatment, for a different reason.**
  `seedMergedMergeRequest`'s merge PUT started failing with `"SHA must be
provided when merging"` once discussion-seeding worked, then `405
Method Not Allowed` once a `sha` was added -- GitLab's mergeability
  check is a separate async step from diff computation.
  `waitForMergeable` polls for `detailed_merge_status == "mergeable"` or
  the legacy `merge_status == "can_be_merged"` before the merge PUT.
- **Surface the response body on every `do` failure, permanently.** The
  fixture's `do` helper only asserted the status code before this; without
  seeing the actual GitLab error, root-causing this would have stayed
  guesswork. Kept as a real diagnostic improvement, not reverted after use.

## Risks / trade-offs

- Confirmed `internal/gitlab/pull_request.go` only ever reads existing
  merge requests and discussions -- it never creates one -- so this
  async-consistency gap is fixture-only. No production code carries the
  same race.
- The 30 s poll timeouts are a guess at "generous enough," not a measured
  bound. If GitLab CE's diff worker is ever slower than that on a loaded
  CI runner, the fixture fails loudly with a clear message rather than
  hanging, which is the safer failure mode either way.
