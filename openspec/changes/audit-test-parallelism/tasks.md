## Root package

- [ ] 1.1 Review `archive_test.go`, `fake_test.go`, `metadata_test.go`, `run_test.go` and add `t.Parallel()` to every test/subtest with no shared mutable state; verify with `go test -race .`
- [ ] 1.2 Review `cli_config_test.go`, `cli_progress_test.go`, `cli_test.go`, `config_test.go`, `mirror_lfs_test.go` — add `t.Parallel()` to subtests that don't call `t.Setenv`, and add a comment at each `t.Setenv`-using test explaining why it stays serial; verify with `go test -race .`

## `internal/forgejo`

- [ ] 2.1 Add `t.Parallel()` to the non-integration tests (`fixture_test.go`, `release_unit_test.go`, and any other unit test with no shared state); verify with `go test -race ./internal/forgejo/...`
- [ ] 2.2 Review the `//go:build integration` suite (`acceptance_test.go`, `container_test.go`, `fixture_search_test.go`, `issue_comments_pagination_test.go`, `issue_pull_request_test.go`, `issue_test.go`, `lister_test.go`, `mirror_lfs_test.go`, `mirror_repo_test.go`, `mirror_test.go`, `release_test.go`, `skip_mirrors_test.go`): add `t.Parallel()` to whichever only read through the shared container's already-authenticated client, and leave a comment on whichever mutate shared container state explaining why they stay serial; verify with `go test -tags=integration -race ./internal/forgejo/...`

## `internal/github`

- [ ] 3.1 Add `t.Parallel()` to the non-integration tests (`release_unit_test.go`, `remote_test.go`, `scope_test.go`, and any other unit test with no shared state); verify with `go test -race ./internal/github/...`
- [ ] 3.2 Review the `//go:build integration` suite (`issue_comments_pagination_test.go`, `issue_test.go`, `lister_test.go`, `release_test.go`): add `t.Parallel()` where independent, comment where it must stay serial; verify with `go test -tags=integration -race ./internal/github/...`

## `internal/gitlab`

- [ ] 4.1 Confirm `concurrency_test.go` and `wiki_snippet_test.go` already follow the pattern this change wants (they're the two existing examples) and add `t.Parallel()` to the remaining non-integration tests (`issue_unit_test.go`, `recorded_issue_test.go`, `recorded_release_test.go`, `recorded_test.go`, `release_unit_test.go`, `update_fixtures_test.go`, and any other unit test with no shared state); verify with `go test -race ./internal/gitlab/...`
- [ ] 4.2 Review the `//go:build integration && gitlab` suite (`acceptance_test.go`, `container_test.go`, `issue_test.go`, `mirror_test.go`, `release_test.go`): add `t.Parallel()` where independent, comment where it must stay serial; verify with `go test -tags=integration -gitlab -race ./internal/gitlab/...`

## Full-suite verification

- [ ] 5.1 Run `go test ./...` and `go test -tags=integration -race ./...` (plus the `gitlab`-tagged lane) end to end and confirm both pass with no new flakiness, then run each suite a second time back to back to catch a race that only shows up intermittently
