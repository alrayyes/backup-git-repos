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
  golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 \
  go "$@"
