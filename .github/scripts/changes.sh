#!/usr/bin/env bash
# Decides which CI jobs run (spec 9.5: build only what changed).
# Writes key=value lines to $GITHUB_OUTPUT (or stdout when run locally):
#   go, rust, ts, proto, openapi, compose = true|false
#   images = JSON array of image names to build
# Usage: changes.sh <base-ref-or-sha> [head]   (empty or all-zero base = everything)
set -euo pipefail

base="${1:-}"
head="${2:-HEAD}"
out="${GITHUB_OUTPUT:-/dev/stdout}"

all=false
if [[ -z "$base" || "$base" =~ ^0+$ ]] || ! git cat-file -e "$base^{commit}" 2>/dev/null; then
  all=true
  files=""
else
  files=$(git diff --name-only "$base"..."$head")
fi
# A change to CI itself re-runs everything.
if grep -qE '^(\.github/|Makefile$)' <<<"$files"; then all=true; fi

changed() { # <regex>
  $all && return 0
  grep -qE "$1" <<<"$files"
}
flag() { if changed "$2"; then echo "$1=true"; else echo "$1=false"; fi; }

GO_SVC='services/(gateway|auth|catalog|search|order)/'
RUST_SVC='services/(inventory|payment|payment-provider-mock)/'
PROTO='^(proto/|libs/(go|rust|ts)/proto/)'
GO="^(libs/go/|go\.work|\.golangci\.yml|$GO_SVC)"
RUST="^(libs/rust/|Cargo\.(toml|lock)|rust-toolchain\.toml|clippy\.toml|$RUST_SVC)"
TS='^(libs/ts/|services/notification/|web/|package(-lock)?\.json|tsconfig\.base\.json|eslint\.config\.mjs|\.prettier)'

{
  flag proto "$PROTO"
  flag go "$GO|$PROTO"
  flag rust "$RUST|$PROTO"
  flag ts "$TS|$PROTO"
  flag openapi '^(services/gateway/api/|redocly\.yaml|\.redocly)'
  flag compose '^(deploy/compose/|\.env\.example)'

  images=()
  for s in gateway auth catalog search order; do
    changed "^(services/$s/|services/go\.Dockerfile|libs/go/)" && images+=("$s")
  done
  for s in inventory payment payment-provider-mock; do
    changed "^(services/$s/|services/rust\.Dockerfile|libs/rust/|Cargo\.(toml|lock)|rust-toolchain\.toml)" && images+=("$s")
  done
  changed '^(services/notification/|services/node\.Dockerfile|libs/ts/|package(-lock)?\.json|tsconfig\.base\.json)' && images+=(notification)
  changed '^(web/|package(-lock)?\.json)' && images+=(web)
  if ((${#images[@]})); then
    printf 'images=[%s]\n' "$(printf '"%s",' "${images[@]}" | sed 's/,$//')"
  else
    echo 'images=[]'
  fi
} >>"$out"
