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

Not yet made -- needs a live container to inspect the actual 400 response
body (the fixture's own `do` helper only asserts status, it doesn't
surface the body on failure) before picking a fix.

## Risks / trade-offs

- If the same `diff_refs`-right-after-create pattern exists in production
  code (not just the fixture), fixing only the fixture would leave a real
  bug in place. Check `internal/gitlab/pull_request.go`'s own review
  comment export path once the root cause is known.
