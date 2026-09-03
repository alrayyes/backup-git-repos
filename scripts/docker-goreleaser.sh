#!/usr/bin/env bash
# Runs goreleaser through the pinned image matching the release workflow's
# own goreleaser-action version (rules/go-releases.md), so a hook and CI
# validate .goreleaser.yaml with the same goreleaser that will actually cut
# the release.
set -euo pipefail

docker run --rm --user "$(id -u):$(id -g)" \
  -v "$(pwd):/src" -w /src \
  --entrypoint goreleaser \
  goreleaser/goreleaser:v2.17.1@sha256:1098a0be4da1780f9616a85f4c5050447b53e3e74804d8017ec1e2bbb1fb697a \
  "$@"
