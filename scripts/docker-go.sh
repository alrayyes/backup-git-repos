#!/usr/bin/env bash
# Runs a `go` subcommand through the pinned image matching go.mod's own `go`
# directive, so the toolchain version a hook runs with is never a question
# the host's package manager gets a vote on (rules/go.md). --user matches
# the host UID/GID so files the container writes (go.mod, the module cache)
# stay owned by the caller, not root.
set -euo pipefail

docker run --rm --user "$(id -u):$(id -g)" \
  -v "$(pwd):/src" -w /src \
  -e GOCACHE=/gocache -v "$HOME/.cache/go-build-docker:/gocache" \
  -e GOMODCACHE=/gomod -v "$HOME/.cache/go-mod-docker:/gomod" \
  golang:1.26.9-bookworm@sha256:d9c68c2c51161e12fd77e4c6320687c9cd86e1af1e3ad6e6cd63ff970641453c \
  go "$@"
