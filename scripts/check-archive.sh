#!/usr/bin/env bash
# Checks that a release archive holds what the AUR package() script and a
# plain download need: the binary, LICENSE, README and a gzipped man page.
# archives.files replaces goreleaser's default list, so dropping one of them
# there passes every other check and only breaks at release time.
set -euo pipefail

archive="${1:?usage: check-archive.sh <archive.tar.gz>}"
listing="$(tar -tzf "$archive" | sed 's,^\./,,')"

missing=0
for want in '^backup-git-repos$' '^LICENSE$' '^README\.md$' '^[^/]+\.1\.gz$'; do
  if ! grep -Eq "$want" <<<"$listing"; then
    echo "check-archive: $archive has nothing matching $want" >&2
    missing=1
  fi
done
exit "$missing"
