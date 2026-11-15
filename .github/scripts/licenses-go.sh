#!/usr/bin/env bash
# Fails if a Go dependency has a license outside licenses.allow (R6).
# Usage: licenses-go.sh <module dir>...
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
allowed=$(grep -v '^#' "$root/licenses.allow" | grep . | paste -sd, -)

for m in "$@"; do
  echo "go-licenses $m"
  if ! out=$(cd "$root/$m" && go-licenses check ./... \
    --ignore github.com/sokolovlev/polka \
    --allowed_licenses="$allowed" 2>&1); then
    # Drop warnings (W...) and the assembly-file lines that follow them.
    grep -vE '^W[0-9]|\.s$' <<<"$out" >&2
    exit 1
  fi
done
echo "go: all licenses allowed"
