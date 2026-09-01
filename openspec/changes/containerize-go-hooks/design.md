## Context

See proposal.md. `rules/go.md`'s own reasoning: "A host package manager
updates `go` and `golangci-lint` independently, and them drifting apart is
a real failure mode too" -- confirmed as a real incident on another
machine, in that same rule file. This repo hasn't hit it yet, but the
hooks currently have no protection against it either.

## Goals / non-goals

**Goals:**

- Every Go-related hook command runs through the pinned image, not the
  host toolchain.
- Local dev experience stays workable -- warm caches keep the added cost
  small after the first run.

**Non-goals:**

- Changing CI's own Go setup (`actions/setup-go` with
  `go-version-file: go.mod` already pins correctly there).
- Containerizing anything beyond the five Go-related jobs this proposal
  names.

## Decisions

Not yet made -- this needs the two-cache-mount gotcha
(`GOCACHE`/`GOLANGCI_LINT_CACHE`, both explicit env vars _and_ mounts,
per `rules/go.md`) verified working end to end on this repo specifically
before merging, not just copied from the rule's example. Also needs a
real measurement of added wall-clock cost on a cold vs. warm cache,
stated in the eventual PR rather than assumed negligible.

## Risks / trade-offs

- Docker becomes a hard dependency for hooks that previously didn't need
  it -- a contributor without Docker running loses `pre-commit`/`pre-push`
  entirely for Go changes, not just a slower run.
- A cold cache on first use adds real wall-clock time; worth measuring
  before deciding whether that's acceptable as-is or wants a "skip if
  Docker isn't running" fallback (which would reintroduce the exact drift
  problem this change exists to close, so likely not the right answer,
  but worth stating explicitly rather than silently deciding either way).
