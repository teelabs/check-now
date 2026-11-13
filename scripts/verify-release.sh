#!/bin/sh
set -eu

dir=dist/release
expected=$(tr -d '[:space:]' <VERSION)
while [ "$#" -gt 0 ]; do
  case "$1" in
    --dir) [ "$#" -ge 2 ] || { echo '--dir requires a value' >&2; exit 2; }; dir=$2; shift 2 ;;
    -h|--help) echo 'Usage: scripts/verify-release.sh [--dir DIR]'; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

cd "$dir"
sha256sum --check --strict SHA256SUMS
for arch in amd64 arm64; do
  archive=check-now-linux-$arch.tar.gz
  sha256sum --check --strict "$archive.sha256"
  contents=$(tar -tzf "$archive")
  for path in check-now plugins check-now.sh check-now.fish VERSION; do
    printf '%s\n' "$contents" | grep -Eq "^${path}(/|$)" || {
      echo "$archive is missing $path" >&2
      exit 1
    }
  done
  bundle_version=$(tar -xOzf "$archive" VERSION)
  [ "$bundle_version" = "$expected" ] || {
    echo "$archive has version $bundle_version; expected $expected" >&2
    exit 1
  }
done
printf 'Release bundles and SHA-256 checksums are valid for %s\n' "$expected"
