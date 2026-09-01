#!/usr/bin/env bash
# Runs golangci-lint through the pinned image matching CI's own
# golangci-lint-action version (rules/go-lint.md), so a hook and CI never
# type-check with a different golangci-lint than each other. Two cache
# mounts, not one -- GOLANGCI_LINT_CACHE for the linter's own result cache,
# GOCACHE (read from the image's own `go env`) for the Go build cache
# underneath it. Skip either and the run still completes, just cold from
# scratch every single time.
set -euo pipefail

docker run --rm --user "$(id -u):$(id -g)" \
  -v "$(pwd):/src" -w /src \
  -e GOCACHE=/gocache -v "$HOME/.cache/go-build-docker:/gocache" \
  -e GOLANGCI_LINT_CACHE=/cache -v "$HOME/.cache/golangci-lint-docker:/cache" \
  golangci/golangci-lint:v2.12.2@sha256:5cceeef04e53efe1470638d4b4b4f5ceefd574955ab3941b2d9a68a8c9ad5240 \
  golangci-lint "$@"
