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

- **Two small wrapper scripts** (`scripts/docker-go.sh`,
  `scripts/docker-golangci-lint.sh`) rather than inlining the full
  `docker run` invocation in each of the five `lefthook.yml` job entries.
  The two images need different cache env vars (`golang` only needs
  `GOCACHE`/`GOMODCACHE`; `golangci-lint` needs `GOCACHE` and
  `GOLANGCI_LINT_CACHE`), so one generic wrapper for both would need a
  branch anyway -- two small scripts read more plainly than one
  parameterized one.
- **Digest-pinned, not just tag-pinned**, matching `dependencies.md`'s
  "Container images: `image:tag@sha256:…`" over `go.md`/`go-lint.md`'s
  own simplified examples (tag-only) -- this repo's own existing
  `hadolint/hadolint` pin already sets that precedent for a raw
  `docker run` invocation, not just Dockerfiles. `golang:1.26.6-bookworm`
  matches `go.mod`'s own `go 1.26.6` directive exactly;
  `golangci/golangci-lint:v2.12.2` matches CI's own
  `golangci-lint-action` version.
- **Dropped the `command -v golangci-lint` fallback entirely**, per the
  proposal -- containerizing removes the reason it existed, and keeping
  it would silently let a stale host binary run again on a machine that
  happens to have one on `PATH`.
- Verified live: both cache mounts genuinely persist across runs. See
  tasks.md 1.6 for the measured cold/warm numbers -- the real added cost
  after the first run is noise-level for three of the five jobs and
  ~8.75 s for `golangci-lint run`, Docker's own per-invocation overhead
  rather than anything cache-related.

## Risks / trade-offs

- Docker becomes a hard dependency for hooks that previously didn't need
  it -- a contributor without Docker running loses `pre-commit`/`pre-push`
  entirely for Go changes, not just a slower run.
- A cold cache on first use adds real wall-clock time; worth measuring
  before deciding whether that's acceptable as-is or wants a "skip if
  Docker isn't running" fallback (which would reintroduce the exact drift
  problem this change exists to close, so likely not the right answer,
  but worth stating explicitly rather than silently deciding either way).
