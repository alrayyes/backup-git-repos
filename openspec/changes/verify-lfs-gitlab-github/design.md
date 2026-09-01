## Context

`internal/gitlab/container_test.go` already boots a pinned, tuned-down GitLab CE container (`runGitLab`, `//go:build integration && gitlab`) for `TestContainerBoots`, and CONTRIBUTING.md documents this as a nightly/`workflow_dispatch`-only lane, never run on a pull request. `internal/forgejo/mirror_lfs_test.go` already proves the same acceptance criteria against Forgejo: a fresh LFS-tracked clone, an update to a legacy (pre-LFS-aware) mirror, and an ordinary repo touching no `lfs/` directory, each checked with `git lfs fsck`. See proposal.md for why GitLab and GitHub need the same proof.

## Goals / non-goals

**Goals:**

- Reuse `runGitLab`'s container harness for a new LFS-specific test file in `internal/gitlab`, following `mirror_lfs_test.go`'s three-scenario shape.
- Confirm (not assume) whether GitLab CE needs any project- or instance-level LFS toggle, the way Forgejo needed `LFS_START_SERVER`.
- Document GitHub's unverified status without inventing a live test for it.

**Non-Goals:**

- No change to `Mirror.syncLFS` or any other production LFS-handling code -- this is test and documentation only.
- No new GitHub-specific test. GitHub.com has no self-hostable server; a live GitHub LFS test is out of scope entirely, not deferred.
- No change to the fast, default `-tags=integration` lane's GitLab/GitHub recorded-fixture tests -- LFS needs a real server round-trip, so it belongs only in the heavy `gitlab`-tagged lane, same as `TestContainerBoots`.

## Decisions

- **New file, not an addition to `container_test.go`.** `mirror_gitlab_lfs_test.go` (or similar), keeping `container_test.go` scoped to "does the container boot" and the new file scoped to LFS, the same separation `internal/forgejo` already has between its own container fixtures and `mirror_lfs_test.go`.
- **Reuse `runGitLab` as-is for the container boot; push the LFS-tracked repo and run the mirror/fsck assertions in the new file**, rather than parameterizing `runGitLab` itself with an LFS flag. Unlike Forgejo, GitLab CE's Omnibus LFS support is controlled at the instance level via `gitlab_rails['lfs_enabled']`, which defaults to `true` -- no config-time toggle is expected to be needed, so there's no reason to thread a flag through the shared boot helper. If implementation finds this default doesn't hold (LFS turned off or gated per-project), add `gitlab_rails['lfs_enabled'] = true;` to `omnibusConfig` rather than inventing a second boot path.
- **Push the LFS-tracked repo over the GitLab REST API + git-http**, mirroring `pushLFSRepo`'s shape in the Forgejo test: create a project via the API, clone it, `git lfs track`, commit a binary file, push. GitLab's clone/push authentication is already exercised by the package's existing `fixture_test.go` and `mirror_test.go`; the new helper reuses that pattern rather than reinventing HTTP auth.
- **Three tests, mirroring Forgejo's exactly**, so the two forges' LFS coverage reads as one story: fresh clone fetches real LFS content (not just pointers), a legacy (plain `git clone --mirror`) mirror backfills its LFS objects on the next `Sync`, and an ordinary non-LFS repo creates no `lfs/` directory.
- **`t.Parallel()` stays off** for these tests, same exception `container_test.go`'s own tests already take: one heavy, shared-cost GitLab CE container per test run, consistent with `rules/go-test.md`'s "shared… fixed external resource" exception. Each test still boots its own container instance (matching the existing per-test `runGitLab` call pattern), so there's no cross-test state to race on, but running LFS tests serially alongside `TestContainerBoots` keeps the nightly lane's total container count predictable rather than however many `t.Parallel()` would try to boot concurrently.
- **GitHub's caveat goes in CONTRIBUTING.md's existing test-lane section**, next to the description of what the default lane's recorded GitHub fixtures do and don't cover, rather than in README.md's user-facing options -- it's a statement about test coverage, not a documented product limitation a user configures around.

## Risks / trade-offs

- [GitLab CE's default LFS setting turns out to be off, or gated per-project rather than instance-wide] → confirmed live during implementation before the test is written as final; if so, add the one-line Omnibus config toggle (or an API call enabling the project's LFS) rather than restructuring the harness.
- [A second full GitLab CE boot (this test's own, on top of `TestContainerBoots`'s) adds meaningful nightly CI time] → acceptable: the lane already excludes itself from every pull request and only runs nightly/on-demand, matching the existing lane's own cost profile.
- [GitLab's LFS batch API behaves differently from Forgejo's around absolute URLs, the way Forgejo's ROOT_URL/host-port coupling did] → mitigated by proving it live rather than assuming parity; if the same class of issue appears, it's the same fix `startWithLFS` used (pin `external_url`/host-port together before the container starts).
