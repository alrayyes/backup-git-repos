#!/usr/bin/env bash
# Proves the Dockerfile still builds -- hadolint reads it as text and never
# actually builds it, so a syntactically fine Dockerfile can still fail to
# build (rules/containers.md). dockers_v2 stages each platform's binary at
# <goos>/<goarch>/<binary> in the build context before invoking `docker
# build`; this reproduces that layout for linux/amd64 only, since one
# platform is enough to prove the Dockerfile itself is sound.
set -euo pipefail

context="$(mktemp -d)"
trap 'rm -rf "$context"' EXIT

mkdir -p "$context/linux/amd64"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -o "$context/linux/amd64/backup-git-repos" ./cmd/backup-git-repos
cp Dockerfile "$context/"

docker build \
  --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 \
  -t backup-git-repos:build-check \
  "$context"
docker rmi backup-git-repos:build-check >/dev/null
