## `internal/gitlab`

- [ ] 1.1 Replace `strings.HasSuffix` + trim with `strings.CutSuffix` at `recorded_issue_test.go:37`
- [ ] 1.2 Add the missing blank line before the bare `return` at `recorded_issue_test.go:42,45,50`
- [ ] 1.3 Add the missing blank line before `return string(out)` at `mirror_test.go:81`
- [ ] 1.4 Verify `golangci-lint run --build-tags integration ./...` and `--build-tags 'integration gitlab' ./...` both report 0 issues
- [ ] 1.5 Verify `go test -tags='integration gitlab' -race -count=1 ./internal/gitlab/...` still passes
