#!/usr/bin/env bash
# Runs goreleaser through the pinned image matching the release workflow's
# own goreleaser-action version (rules/go-releases.md), so a hook and CI
# validate .goreleaser.yaml with the same goreleaser that will actually cut
# the release.
set -euo pipefail

# The image bundles a fixed Go version tied to when it was built, and pins
# GOTOOLCHAIN=local -- a go.mod bump past that version fails outright
# instead of fetching the newer toolchain the way a plain `go` install
# would. GOTOOLCHAIN=auto restores that standard behavior instead of
# re-pinning the image every time go.mod's directive moves.
docker run --rm --user "$(id -u):$(id -g)" \
  -e GOTOOLCHAIN=auto -e HOME=/tmp \
  -v "$(pwd):/src" -w /src \
  --entrypoint goreleaser \
  goreleaser/goreleaser:v2.17.1@sha256:1098a0be4da1780f9616a85f4c5050447b53e3e74804d8017ec1e2bbb1fb697a \
  "$@"
