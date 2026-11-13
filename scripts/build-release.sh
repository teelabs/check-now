#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=$(tr -d '[:space:]' <"$root/VERSION")
out_dir=$root/dist/release

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) [ "$#" -ge 2 ] || { echo '--version requires a value' >&2; exit 2; }; version=$2; shift 2 ;;
    --out-dir) [ "$#" -ge 2 ] || { echo '--out-dir requires a value' >&2; exit 2; }; out_dir=$2; shift 2 ;;
    -h|--help) echo 'Usage: scripts/build-release.sh [--version VERSION] [--out-dir DIR]'; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

case "$version" in
  ''|*[!0-9.]*|.*|*..*|*.) echo "invalid version: $version" >&2; exit 1 ;;
esac
old_ifs=$IFS
IFS=.
set -- $version
IFS=$old_ifs
[ "$#" -eq 3 ] || { echo "invalid version: $version" >&2; exit 1; }

command -v go >/dev/null 2>&1 || { echo 'go is required to build release bundles' >&2; exit 1; }
command -v tar >/dev/null 2>&1 || { echo 'tar is required' >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { echo 'sha256sum is required' >&2; exit 1; }
command -v realpath >/dev/null 2>&1 || { echo 'realpath is required' >&2; exit 1; }

case "$out_dir" in /*) ;; *) out_dir=$root/$out_dir ;; esac
out_dir=$(realpath -m "$out_dir")
default_out_dir=$(realpath -m "$root/dist/release")
case "$out_dir" in
  /|"$root") echo "refusing unsafe output directory: $out_dir" >&2; exit 1 ;;
  "$root"/*)
    [ "$out_dir" = "$default_out_dir" ] || {
      echo "refusing output directory inside the checkout: $out_dir" >&2
      exit 1
    }
    ;;
esac
if [ "$out_dir" != "$default_out_dir" ] && [ -e "$out_dir" ]; then
  [ -d "$out_dir" ] || { echo "refusing non-directory output path: $out_dir" >&2; exit 1; }
  if [ -n "$(find "$out_dir" -mindepth 1 -maxdepth 1 -print -quit)" ]; then
    echo "refusing non-empty output directory: $out_dir" >&2
    exit 1
  fi
fi
rm -rf "$out_dir"
mkdir -p "$out_dir"
tmp_root=$(mktemp -d "${TMPDIR:-/tmp}/check-now-release.XXXXXX")
trap 'rm -rf "$tmp_root"' EXIT HUP INT TERM

for arch in amd64 arm64; do
  stage=$tmp_root/linux-$arch
  mkdir -p "$stage/plugins"
  CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X main.version=$version" \
    -o "$stage/check-now" "$root/cmd/check-now"
  cp -R "$root/plugins/." "$stage/plugins/"
  cp "$root/check-now.sh" "$root/check-now.fish" "$stage/"
  printf '%s\n' "$version" >"$stage/VERSION"
  archive=check-now-linux-$arch.tar.gz
  tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
    -czf "$out_dir/$archive" -C "$stage" check-now plugins check-now.sh check-now.fish VERSION
  (cd "$out_dir" && sha256sum "$archive" >"$archive.sha256")
done

(cd "$out_dir" && sha256sum check-now-linux-amd64.tar.gz check-now-linux-arm64.tar.gz >SHA256SUMS)
printf 'Built check-now %s release bundles in %s\n' "$version" "$out_dir"
