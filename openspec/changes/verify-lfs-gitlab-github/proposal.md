## Why

`Mirror.syncLFS` (added in #5) is proven end-to-end against Forgejo, but GitLab and GitHub were never exercised live for LFS specifically -- GitLab's LFS behavior only shows up under the heavy, nightly only GitLab CE container, and GitHub.com has no self-hostable equivalent to boot at all. Both adapters authenticate the way #5 found LFS actually needs (HTTP Basic, not Forgejo's original `"token <t>"`), so the behavior is _expected_ to work, but nobody has mirrored an LFS-tracked GitLab or GitHub repository and run `git lfs fsck` against the result to confirm it.

## What changes

- Add a live LFS integration test to `internal/gitlab`'s nightly, real-container lane (`-tags='integration gitlab'`), mirroring `internal/forgejo/mirror_lfs_test.go`'s pattern: fresh clone fetches LFS content, an update to a legacy (pre-#5) mirror backfills it, and an ordinary repo creates no `lfs/` directory.
- Document GitHub.com's LFS status explicitly in README/CONTRIBUTING as "expected to work, unverified against a live server" -- no self-hostable GitHub instance exists to test against, so this is a documentation change, not a new test.
- No production code changes: this verifies and documents existing `Mirror.syncLFS` behavior, it does not alter it.

## Capabilities

### New capabilities

None.

### Modified capabilities

None -- `Mirror.syncLFS`'s externally observable behavior is unchanged. This change adds test coverage confirming that behavior already holds against GitLab, and documents its unverified status against GitHub; no requirement is being added, removed, or altered. `skip_specs: true` is set in this change's `.openspec.yaml` accordingly.

## Impact

- `internal/gitlab`: new LFS-specific container test file (extends the existing `container_test.go` harness), possibly a GitLab LFS-enablement toggle if GitLab CE doesn't default it on for a fresh project.
- `CONTRIBUTING.md`: test-lane description gains any GitLab LFS setup note; README and/or CONTRIBUTING gains the GitHub "unverified" caveat.
- No changes to `internal/github`, `metadata.go`, `cli.go`, or any production LFS-handling code.
