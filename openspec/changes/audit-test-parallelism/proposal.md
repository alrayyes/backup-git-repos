## Why

`go test ./...` runs the suite serially by default. rules/go-test.md requires
`t.Parallel()` in every test whose scenario is independent of its siblings,
but a repo-wide check found 43 files with `func Test` and only 3 calling
`t.Parallel()` anywhere (`internal/gitlab/concurrency_test.go`,
`internal/gitlab/wiki_snippet_test.go`, `mirror_lfs_test.go`). The gap costs
wall-clock time on every local run and every CI job, for no reason beyond it
never having been added. Tracked as GitHub issue #102.

## What changes

- Add `t.Parallel()` to every test function and subtest whose scenario
  doesn't share mutable state or a fixed external resource with its
  siblings.
- Leave serial, with a comment explaining why, any test that genuinely can't
  go parallel: anything calling `t.Setenv` (which panics under
  `t.Parallel()`), the `//go:build integration` suites that share one
  booted Forgejo/GitLab container, and any test binding a fixed
  `httptest` port.
- No production code changes and no behavior change outside how the test
  suite executes.

## Capabilities

### New capabilities

None.

### Modified capabilities

None — this changes how tests run, not what the CLI does. No spec-level
requirement changes.

## Impact

- Every `_test.go` file in the module (root package, `internal/forgejo`,
  `internal/github`, `internal/gitlab`) is a candidate for review.
- `go test ./...` and `go test -tags=integration -race ./...` must both
  still pass after the change, with no new flakiness from tests that are
  now racing on state they didn't previously share.
- No dependency, API, or CLI-flag changes.
