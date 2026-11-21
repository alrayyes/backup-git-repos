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
  golang:1.26.11-bookworm@sha256:dc40a13b61304647ed7e5d581d246b86413dfc29d63c6d018310feb93cd98fc0 \
  go "$@"
