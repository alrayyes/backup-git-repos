## `internal/gitlab`

- [x] 1.1 Replace `strings.HasSuffix` + trim with `strings.CutSuffix` at `recorded_issue_test.go:37`
- [x] 1.2 Add the missing blank line before the bare `return` at `recorded_issue_test.go:42,45,50`
- [x] 1.3 Add the missing blank line before `return string(out)` at `mirror_test.go:81`
- [x] 1.4 Verify `golangci-lint run --build-tags integration ./...` and `--build-tags 'integration gitlab' ./...` both report 0 issues. golangci-lint reported more findings than the original 5 once the first were fixed -- it doesn't report every issue in a file in one pass, so this took several fix-and-rerun rounds until stable. Also fixed: 7 more `nlreturn` findings in `recorded_test.go` and `update_fixtures_test.go`, not visible in the original scan.
- [x] 1.5 Verify `go test -tags='integration gitlab' -race -count=1 ./internal/gitlab/...` still passes. Verified the fast recorded-fixture subset (`-run 'TestRecorded|TestUpdateFixtures'`) directly; the full container-booting suite is separately blocked by #123.
