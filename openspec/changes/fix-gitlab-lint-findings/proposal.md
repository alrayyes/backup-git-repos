## Why

`golangci-lint run --build-tags 'integration gitlab'` (and the plain
`--build-tags integration` run, for two of the same findings) reports 5
pre-existing lint issues in `internal/gitlab`'s container-tagged test
files. None of the CI-required checks catch them today: the
`golangci-lint` job only runs default build tags, so they've sat
unnoticed. Tracked as GitHub issue #121, found while auditing #102.

## What changes

- `internal/gitlab/recorded_issue_test.go:37` -- replace
  `strings.HasSuffix` + a manual trim with `strings.CutSuffix`
  (`modernize`).
- `internal/gitlab/recorded_issue_test.go:42,45,50` -- add the blank line
  `nlreturn` wants before each bare `return`.
- `internal/gitlab/mirror_test.go:81` -- same `nlreturn` fix on
  `return string(out)`.
- No behavior change: these are style-only findings in test helper code.

## Capabilities

### New capabilities

None.

### Modified capabilities

None -- lint-only, no requirement changes. `skip_specs: true` set in this
change's `.openspec.yaml`.

## Impact

- `internal/gitlab/recorded_issue_test.go`, `internal/gitlab/mirror_test.go`.
- `golangci-lint run --build-tags integration` and
  `--build-tags 'integration gitlab'` must both go clean.
- `go test -tags='integration gitlab' -race -count=1 ./internal/gitlab/...`
  must still pass.
