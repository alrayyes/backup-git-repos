#!/usr/bin/env bash
# Reads a failed `nix build` log and, if it holds a fixed-output hash mismatch,
# rewrites flake.nix's vendorHash to the hash Nix says it computed.
#
# Exit 0: flake.nix updated. Exit 3: nothing to fix (no mismatch in the log, the
# hash is already what Nix reported, or the reported value isn't a sha256 SRI
# hash), so the build failed for some other reason and a human should look.
set -euo pipefail

log="${1:?usage: update-vendor-hash.sh <nix-build.log> [flake.nix]}"
flake="${2:-flake.nix}"

# Only a well-formed sha256 SRI hash is ever written into flake.nix.
got="$(sed -n 's/^[[:space:]]*got:[[:space:]]*\(sha256-[A-Za-z0-9+/]\{43\}=\)[[:space:]]*$/\1/p' "$log" | head -n1)"
if [ -z "$got" ]; then
  echo "update-vendor-hash: no hash mismatch in $log" >&2
  exit 3
fi

current="$(sed -n 's/.*vendorHash = "\(sha256-[^"]*\)";.*/\1/p' "$flake" | head -n1)"
if [ -z "$current" ]; then echo "update-vendor-hash: no vendorHash line in $flake" >&2; exit 3; fi
if [ "$current" = "$got" ]; then
  echo "update-vendor-hash: $flake already has $got" >&2
  exit 3
fi

# First match only, same as $current: the script assumes one package in the flake.
sed -i "0,/vendorHash = \"[^\"]*\";/s||vendorHash = \"$got\";|" "$flake"
echo "update-vendor-hash: $current -> $got"
