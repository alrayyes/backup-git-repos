## Context

See proposal.md. All 5 findings are pre-existing on `main` (confirmed by
running the same `golangci-lint` invocation against a stash of the
working tree). Nothing about #102's `t.Parallel()` audit introduced them.

## Goals / non-goals

**Goals:**

- `golangci-lint run --build-tags integration` and
  `--build-tags 'integration gitlab'` both report 0 issues in
  `internal/gitlab`.

**Non-goals:**

- Wiring these build tags into the CI `golangci-lint` job. That's a
  separate, bigger decision (running full container-tagged lint on every
  pull request) not implied by fixing 5 existing findings.

## Decisions

- **Apply the tool's own suggested fix, not a hand-rolled equivalent.**
  `CutSuffix` and the `nlreturn` blank line are both mechanical rewrites
  with one obviously correct form; no alternative worth weighing.

## Risks / trade-offs

- None. Pure style changes in test helper code, no behavior to regress.
