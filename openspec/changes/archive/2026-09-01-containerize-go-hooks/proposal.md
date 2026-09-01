## Why

`rules/go.md` and `rules/go-lint.md` both specify running every Go-related
hook command through the pinned `golang:<version>-bookworm` and
`golangci/golangci-lint:<version>` Docker images, so the toolchain version
a hook runs with is never a question the host's package manager gets a
vote on. Found during a full CLAUDE.md/rules/ audit: `lefthook.yml`
currently runs `go-mod-fmt`, `go-mod-tidy`, `go-test`, `golangci-lint-fmt`,
and `golangci-lint` against whatever `go`/`golangci-lint` happens to be on
`PATH`, with no version pin. Tracked as GitHub issue #132.

## What changes

- Containerize all five Go-related lefthook jobs, matching `go.md`'s exact
  invocation shape: `docker run --user "$(id -u):$(id -g)"`, both cache
  mounts (`GOCACHE`, `GOLANGCI_LINT_CACHE`) as explicit env vars and
  volumes.
- Drop the current `command -v golangci-lint` skip-if-missing fallback --
  containerizing removes the reason it existed.
- Update `CONTRIBUTING.md`'s "Getting set up" section if the local
  toolchain requirement changes (Docker becomes required for hooks, not
  just the integration test suites and hadolint).

## Capabilities

### New capabilities

None.

### Modified capabilities

None -- tooling-only, no requirement changes. `skip_specs: true` set in
this change's `.openspec.yaml`.

## Impact

- `lefthook.yml`: `go-mod-fmt`, `go-mod-tidy`, `go-test`,
  `golangci-lint-fmt`, `golangci-lint` jobs (both `pre-commit` and
  `pre-push` sections).
- Local dev experience: Docker becomes a hard dependency for every Go
  hook, and a cold cache adds real wall-clock time on the first run after
  this lands -- worth measuring and stating, not assuming negligible.
- CI's own Go job already pins via `actions/setup-go`'s
  `go-version-file: go.mod` and is out of scope here; this is about local
  hook parity.
