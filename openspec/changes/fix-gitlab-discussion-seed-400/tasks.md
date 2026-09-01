## `internal/gitlab`

- [ ] 1.1 Reproduce against a live GitLab CE 19.2.2 container and capture the actual 400 response body (the fixture's `do` helper doesn't currently surface it on failure -- add a temporary print or curl the same request by hand)
- [ ] 1.2 Root-cause the rejection (stale `diff_refs`, a missing required field, an API shape change in 19.2) and fix `seedOpenMergeRequestWithReviewComment` in `internal/gitlab/fixture_test.go`, with a comment at the call site explaining why
- [ ] 1.3 Check whether the same pattern exists in production code (`internal/gitlab/pull_request.go`'s review comment export path); fix there too if so
- [ ] 1.4 Verify `go test -tags='integration gitlab' -race -timeout=35m ./internal/gitlab/...` passes end to end
