## `lefthook.yml`

- [ ] 1.1 Containerize `go-mod-fmt` (pre-push) through `golang:<pinned>-bookworm`
- [ ] 1.2 Containerize `go-mod-tidy` (pre-push) through `golang:<pinned>-bookworm`
- [ ] 1.3 Containerize `go-test` (pre-push) through `golang:<pinned>-bookworm`
- [ ] 1.4 Containerize `golangci-lint-fmt` (pre-commit) and `golangci-lint` (pre-push) through `golangci/golangci-lint:<pinned>`, dropping the `command -v` fallback
- [ ] 1.5 Verify both cache mounts (`GOCACHE`, `GOLANGCI_LINT_CACHE`) actually persist across runs -- confirm a second run is fast, not cold every time
- [ ] 1.6 Measure and state added wall-clock cost (cold and warm cache) in the PR description
- [ ] 1.7 Update `CONTRIBUTING.md`'s "Getting set up" section if Docker becomes a new local requirement for hooks
