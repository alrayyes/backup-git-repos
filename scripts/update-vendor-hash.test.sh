#!/usr/bin/env bash
# Tests for update-vendor-hash.sh: feed it nix build logs and a flake.nix and
# check what it does to the vendorHash line.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
script="$here/update-vendor-hash.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

old='sha256-l/ryhOWGGzeMZpfgXVRO1fHSTWpjzcu3YUdKaKe7F0o='
new='sha256-vI3DQ6zmaZRy8UsYyGf1x1J2e2vScbTj3W+VhlWfV50='
fails=0

flake() {
  printf '          src = ./.;\n          vendorHash = "%s";\n          ldflags = [ "-s" ];\n' "$1" >"$tmp/flake.nix"
}
check() { # name, want-exit, got-exit
  if [ "$2" != "$3" ]; then echo "FAIL $1: exit $3, want $2" >&2; fails=$((fails + 1)); fi
}

# 1. A mismatch in the log rewrites the hash and nothing else.
flake "$old"
cat >"$tmp/mismatch.log" <<LOG
error: hash mismatch in fixed-output derivation '/nix/store/x-backup-git-repos-1.0.0-go-modules.drv':
         specified: $old
            got:    $new
LOG
rc=0; "$script" "$tmp/mismatch.log" "$tmp/flake.nix" >/dev/null || rc=$?
check "mismatch exit" 0 "$rc"
grep -q "vendorHash = \"$new\";" "$tmp/flake.nix" || { echo "FAIL mismatch: hash not updated" >&2; fails=$((fails + 1)); }
grep -q 'ldflags = \[ "-s" \];' "$tmp/flake.nix" || { echo "FAIL mismatch: other lines changed" >&2; fails=$((fails + 1)); }

# 2. A log with no mismatch (some other build failure) changes nothing and says so.
flake "$old"
printf 'error: builder for foo failed with exit code 1\n' >"$tmp/other.log"
rc=0; "$script" "$tmp/other.log" "$tmp/flake.nix" >/dev/null 2>&1 || rc=$?
check "no-mismatch exit" 3 "$rc"
grep -q "vendorHash = \"$old\";" "$tmp/flake.nix" || { echo "FAIL no-mismatch: flake.nix changed" >&2; fails=$((fails + 1)); }

# 3. A mismatch whose got: equals the current hash is not a fix: refuse, don't loop.
flake "$new"
cat >"$tmp/same.log" <<LOG
         specified: $old
            got:    $new
LOG
rc=0; "$script" "$tmp/same.log" "$tmp/flake.nix" >/dev/null 2>&1 || rc=$?
check "already-correct exit" 3 "$rc"

# 4. A malformed got: value is rejected rather than written into flake.nix.
flake "$old"
printf '         specified: %s\n            got:    not-a-hash"; rm -rf /\n' "$old" >"$tmp/bad.log"
rc=0; "$script" "$tmp/bad.log" "$tmp/flake.nix" >/dev/null 2>&1 || rc=$?
check "malformed exit" 3 "$rc"
grep -q "vendorHash = \"$old\";" "$tmp/flake.nix" || { echo "FAIL malformed: flake.nix changed" >&2; fails=$((fails + 1)); }

[ "$fails" -eq 0 ] && echo "update-vendor-hash: all tests passed"
exit "$fails"
