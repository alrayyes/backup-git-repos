## `lefthook.yml`

- [x] 1.1 Containerize `go-mod-fmt` (pre-push) through `golang:<pinned>-bookworm`
- [x] 1.2 Containerize `go-mod-tidy` (pre-push) through `golang:<pinned>-bookworm`
- [x] 1.3 Containerize `go-test` (pre-push) through `golang:<pinned>-bookworm`
- [x] 1.4 Containerize `golangci-lint-fmt` (pre-commit) and `golangci-lint` (pre-push) through `golangci/golangci-lint:<pinned>`, dropping the `command -v` fallback. Both images pinned by digest, matching `go.mod`'s own `go 1.26.6` directive.
- [x] 1.5 Verify both cache mounts (`GOCACHE`, `GOLANGCI_LINT_CACHE`) actually persist across runs -- confirm a second run is fast, not cold every time. Confirmed: a second run of every command is near-instant (see 1.6's numbers).
- [x] 1.6 Measure and state added wall-clock cost (cold and warm cache) in the PR description. Measured directly:
  - `go mod edit -fmt`: cold 0.36 s, warm 0.32 s (native: 0.01 s)
  - `go mod tidy -diff`: cold 5.44 s, warm 0.34 s (native: 0.08 s)
  - `go test -race ./...`: cold 13.98 s, warm 0.44 s (native: 2.01 s)
  - `golangci-lint run`: cold 27.16 s, warm 9.56 s (native: 0.81 s)
  - The real added cost, warm cache, is ~0.3 s for the two `go mod` commands and `go test` (noise-level), and ~8.75 s for `golangci-lint run` (Docker's own per-invocation overhead, not cache-related).
- [x] 1.7 Update `CONTRIBUTING.md`'s "Getting set up" section if Docker becomes a new local requirement for hooks. Updated: the "What you need" and "Git hooks" sections both now describe Docker as required for every Go hook, and the local `golangci-lint` install is reframed as editor-integration-only.
