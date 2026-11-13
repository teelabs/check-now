#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
temp=$(mktemp -d "${TMPDIR:-/tmp}/check-now-rollback-test.XXXXXX")
trap 'rm -rf "$temp"' EXIT HUP INT TERM
mkdir -p "$temp/mock-bin" "$temp/prefix/bin" "$temp/prefix/share/check-now" "$temp/payload/plugins"
printf 'old binary\n' >"$temp/prefix/bin/check-now"
printf 'old shared data\n' >"$temp/prefix/share/check-now/keep"
printf 'new binary\n' >"$temp/payload/check-now"
printf 'new plugins\n' >"$temp/payload/plugins/example"
printf 'new shell\n' >"$temp/payload/check-now.sh"
printf 'new fish\n' >"$temp/payload/check-now.fish"
printf '1.2.3\n' >"$temp/payload/VERSION"
chmod +x "$temp/payload/check-now"
tar -czf "$temp/check-now-linux-amd64.tar.gz" -C "$temp/payload" .
(cd "$temp" && sha256sum check-now-linux-amd64.tar.gz >check-now-linux-amd64.tar.gz.sha256)

cat >"$temp/mock-bin/curl" <<'MOCK'
#!/bin/sh
set -eu
out=
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then out=$2; shift 2; else shift; fi
done
case "$out" in
  */check-now-linux-amd64.tar.gz) cp "$CHECK_NOW_FIXTURE/check-now-linux-amd64.tar.gz" "$out" ;;
  */check-now-linux-amd64.tar.gz.sha256) cp "$CHECK_NOW_FIXTURE/check-now-linux-amd64.tar.gz.sha256" "$out" ;;
  *) exit 2 ;;
esac
MOCK
cat >"$temp/mock-bin/uname" <<'MOCK'
#!/bin/sh
case "$1" in -m) printf 'x86_64\n' ;; -s) printf 'Linux\n' ;; *) exit 2 ;; esac
MOCK
cat >"$temp/mock-bin/mv" <<'MOCK'
#!/bin/sh
case "$2" in "$CHECK_NOW_PREFIX"/.check-now-backup.*/share-check-now) exit 73 ;; esac
if [ "${CHECK_NOW_FAIL_RESTORE:-false}" = true ]; then
  case "$1:$2" in "$CHECK_NOW_PREFIX"/.check-now-backup.*/check-now:"$CHECK_NOW_PREFIX"/bin/check-now) exit 74 ;; esac
fi
exec /usr/bin/mv "$@"
MOCK
chmod +x "$temp/mock-bin/curl" "$temp/mock-bin/uname" "$temp/mock-bin/mv"

if CHECK_NOW_FIXTURE="$temp" CHECK_NOW_PREFIX="$temp/prefix" HOME="$temp/home" PATH="$temp/mock-bin:$PATH" \
  sh "$repo/distribution/install.sh" --version 1.2.3 --prefix "$temp/prefix" >"$temp/output" 2>&1; then
  printf 'expected simulated shared-file backup failure\n' >&2
  exit 1
fi
[ "$(cat "$temp/prefix/bin/check-now")" = 'old binary' ] || {
  printf 'previous binary was not restored\n' >&2
  exit 1
}
[ "$(cat "$temp/prefix/share/check-now/keep")" = 'old shared data' ] || {
  printf 'previous shared data was lost during rollback\n' >&2
  exit 1
}
printf 'installer rollback preserves the previous installation\n'

mkdir -p "$temp/prefix-failed-restore/bin" "$temp/prefix-failed-restore/share/check-now"
printf 'old binary\n' >"$temp/prefix-failed-restore/bin/check-now"
printf 'old shared data\n' >"$temp/prefix-failed-restore/share/check-now/keep"
if CHECK_NOW_FIXTURE="$temp" CHECK_NOW_PREFIX="$temp/prefix-failed-restore" CHECK_NOW_FAIL_RESTORE=true HOME="$temp/home" PATH="$temp/mock-bin:$PATH" \
  sh "$repo/distribution/install.sh" --version 1.2.3 --prefix "$temp/prefix-failed-restore" >"$temp/failed-restore-output" 2>&1; then
  printf 'expected simulated backup restoration failure\n' >&2
  exit 1
fi
grep -Fq 'rollback incomplete; previous files remain in ' "$temp/failed-restore-output" || {
  printf 'installer did not report retained recovery files\n' >&2
  exit 1
}
set -- "$temp/prefix-failed-restore"/.check-now-backup.*/check-now
[ -f "$1" ] && [ "$(cat "$1")" = 'old binary' ] || {
  printf 'previous binary was not retained after restoration failure\n' >&2
  exit 1
}
printf 'installer retains recovery files when rollback cannot restore them\n'
