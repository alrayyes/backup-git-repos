## Context

See proposal.md for the motivation and the current counts. The suite spans
the root package and `internal/forgejo`, `internal/github`, `internal/gitlab`
— unit tests, HTTP-fixture tests, and `//go:build integration` container
tests. Not every serial test is an oversight; some can't go parallel at all.

## Goals / non-goals

**Goals:**

- Every independent test/subtest calls `t.Parallel()`.
- Every test left serial has a comment at the call site naming the reason,
  so a future pass doesn't "fix" it back to parallel.

**Non-Goals:**

- Restructuring tests to _become_ independent (for example, splitting a test that
  shares a fixture on purpose). This only adds `t.Parallel()` where
  independence already exists.
- Changing what any test asserts, or touching production code.

## Decisions

- **File-by-file judgment, not a blanket `sed`.** A mechanical
  find-and-replace would add `t.Parallel()` to tests that panic
  (`t.Setenv`) or race (shared container, bound port). Each of the 40
  non-parallel files gets read and classified individually.
- **Known serial-by-necessity categories, decided up front so they aren't
  re-litigated per file:**
  - Any test calling `t.Setenv` — `cli_config_test.go`,
    `cli_progress_test.go`, `cli_test.go`, `config_test.go`,
    `mirror_lfs_test.go` (root-level `t.Setenv` calls; `t.Parallel()` still
    applies to sibling subtests in the same file that don't call it).
  - `//go:build integration` suites that share one booted Forgejo/GitLab
    container per package — `internal/forgejo/container_test.go`,
    `internal/gitlab/container_test.go`, and any other integration test in
    those packages that mutates or depends on shared container state rather
    than just reading through the already-authenticated client.
  - Any test binding a fixed `httptest` port rather than
    `httptest.NewServer`'s ephemeral one.
- **Outer test and subtests both get `t.Parallel()` where both are
  independent** — calling it only on the subtest still serializes the outer
  tests against each other; rules/go-test.md asks for both levels.

## Risks / trade-offs

- [Newly parallel tests exposed a latent data race that serial execution
  was masking] → run `go test -race ./...` and
  `go test -tags=integration -race ./...` after the change, not just
  `go vet`; fix or leave serial (with a comment) whatever `-race` flags.
- [A test misclassified as independent shares package-level mutable state
  through a global or a fixture directory] → read the full file, not just
  its test function signature, before adding `t.Parallel()` — the shared
  GitLab CE / Forgejo container tests are the likeliest place for this.
- [Reviewer can't tell "left serial because it's fine to" from "left serial
  because nobody got to it"] → the comment-at-call-site requirement from the
  proposal covers this; a file changed with no `t.Parallel()` and no
  comment is a signal the pass was incomplete.
