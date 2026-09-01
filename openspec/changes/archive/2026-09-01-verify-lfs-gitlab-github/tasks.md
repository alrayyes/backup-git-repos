## Confirm the GitLab LFS assumption

- [ ] 1.1 Boot `runGitLab` locally (`go test -tags='integration gitlab' -run TestContainerBoots ./internal/gitlab/...`) and, against the running container, create a project and push an LFS-tracked file by hand (or a throwaway script) to confirm GitLab CE's default `gitlab_rails['lfs_enabled']` actually accepts the LFS batch API with no extra config -- verify by checking the project's LFS objects appear via the API or `git lfs fsck` on a fresh clone.
- [ ] 1.2 If step 1.1 shows LFS is off or gated, add the minimal Omnibus config or API call needed and note the change against design.md's stated assumption; if it confirms the default works, no code change needed here.

## GitLab LFS integration test

- [ ] 2.1 Add `internal/gitlab/mirror_gitlab_lfs_test.go` (`//go:build integration && gitlab`, `package gitlab_test`) with a `pushLFSRepo`-equivalent helper that creates a GitLab project via the API, clones it, `git lfs track`s a binary file, commits, and pushes -- verify by running the new test file alone and confirming the push succeeds before writing assertions against it.
- [ ] 2.2 Add `TestMirrorSyncFetchesLFSContent` (fresh clone fetches real LFS object content) and verify with `go test -tags='integration gitlab' -run TestMirrorSyncFetchesLFSContent ./internal/gitlab/...` passing, asserting via `git lfs fsck` the same way `internal/forgejo/mirror_lfs_test.go` does.
- [ ] 2.3 Add `TestMirrorSyncFetchesLFSContentOnUpdate` (a legacy `git clone --mirror` with no `lfs/` dir gets backfilled by a subsequent `Mirror.Sync`) and verify it passes under the same build tags.
- [ ] 2.4 Add `TestMirrorSyncSkipsLFSForOrdinaryRepo` (a non-LFS repo mirrors with no `lfs/` directory created) reusing the package's existing non-LFS fixture rather than `runGitLab`'s LFS-pushing helper, and verify it passes.
- [ ] 2.5 Run the full nightly lane locally (`go test -tags='integration gitlab' -race ./internal/gitlab/...`) and verify every test, old and new, passes together with no flakiness across two consecutive runs.

## Documentation

- [ ] 3.1 Update CONTRIBUTING.md's test-lane section to mention the new GitLab LFS coverage in the `gitlab`-tagged nightly lane, including any config prerequisite step 1.2 introduced, and verify by re-reading the section for accuracy against the actual test file.
- [ ] 3.2 Add GitHub's "expected to work, unverified against a live server" caveat to CONTRIBUTING.md (or README.md if it's a more natural fit once both are open) next to the existing description of what the default lane's recorded GitHub fixtures do and don't prove, and verify by reading the surrounding section for consistency with #92's third acceptance criterion.

## Review

- [ ] 4.1 Review the diff end to end -- new test file, any container/config change, and the documentation updates -- confirming it matches design.md's decisions and this task list's completed items.
