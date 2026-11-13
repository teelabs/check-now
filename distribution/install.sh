#!/bin/sh
set -eu

REPOSITORY=https://github.com/teelabs/check-now
PREFIX=${CHECK_NOW_PREFIX:-"${HOME:-}/.local"}
VERSION=latest
SHELL_HOOK=false
SHELL_COMPLETION=false
SHELL_NAME=auto

usage() {
  cat <<'EOF'
Usage: install.sh [--version latest|X.Y.Z] [--prefix DIR]
                  [--shell NAME] [--shell-hook] [--shell-completion]

Downloads a prebuilt Linux release from GitHub and verifies its SHA-256 checksum.
Supported targets are Linux amd64 and arm64. The default prefix is ~/.local.
Shell integration is optional and never runs an update command.
EOF
}

die() { printf 'check-now installer: %s\n' "$*" >&2; exit 1; }

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) [ "$#" -ge 2 ] || die '--version requires latest or X.Y.Z'; VERSION=$2; shift 2 ;;
    --prefix) [ "$#" -ge 2 ] || die '--prefix requires a directory'; PREFIX=$2; shift 2 ;;
    --shell) [ "$#" -ge 2 ] || die '--shell requires a name'; SHELL_NAME=$2; shift 2 ;;
    --shell-hook) SHELL_HOOK=true; shift ;;
    --shell-completion) SHELL_COMPLETION=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

case "$PREFIX" in /*) ;; *) die 'prefix must be an absolute path' ;; esac
[ "$PREFIX" != / ] || die 'prefix must not be /'
case "$PREFIX" in */../*|*/..|*/./*|*/.) die 'prefix must not contain dot path components' ;; esac

if [ "$SHELL_NAME" = auto ]; then SHELL_NAME=$(basename "${SHELL:-sh}"); fi
if [ "$SHELL_NAME" = pwsh ]; then SHELL_NAME=powershell; fi
case "$SHELL_NAME" in bash|zsh|fish|ksh|mksh|dash|sh|powershell) ;; *) die "unsupported shell: $SHELL_NAME" ;; esac
if [ "$SHELL_COMPLETION" = true ]; then
  case "$SHELL_NAME" in bash|zsh|fish) ;; *) die "completion is unsupported for $SHELL_NAME" ;; esac
fi
if [ "$SHELL_HOOK" = true ] && [ "$SHELL_NAME" = powershell ]; then
  die 'PowerShell hook setup is not supported; add check-now shell to $PROFILE manually'
fi
if [ "$SHELL_HOOK" = true ] && [ "$SHELL_NAME" = sh ]; then SHELL_NAME=dash; fi

case "$VERSION" in
  latest) release=latest ;;
  v[0-9]*.[0-9]*.[0-9]*) release=$VERSION ;;
  [0-9]*.[0-9]*.[0-9]*) release=v$VERSION ;;
  *) die 'version must be latest or a semantic version such as 1.2.3' ;;
esac
if [ "$release" != latest ]; then
  version=${release#v}
  case "$version" in *[!0-9.]*|.*|*..*|*.) die 'invalid semantic version' ;; esac
  old_ifs=$IFS
  IFS=.
  set -- $version
  IFS=$old_ifs
  [ "$#" -eq 3 ] || die 'version must contain exactly three numeric components'
fi

machine=$(uname -m)
case "$machine" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported Linux architecture: $machine" ;;
esac
[ "$(uname -s)" = Linux ] || die 'this installer supports Linux only'

download() {
  if command -v curl >/dev/null 2>&1; then
    curl --fail --location --silent --show-error "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$1" -O "$2"
  else
    die 'install curl or wget to download the release'
  fi
}

command -v tar >/dev/null 2>&1 || die 'tar is required'
command -v sha256sum >/dev/null 2>&1 || die 'sha256sum is required'
command -v mktemp >/dev/null 2>&1 || die 'mktemp is required'

temp=$(mktemp -d "${TMPDIR:-/tmp}/check-now-install.XXXXXX")
trap 'rm -rf "$temp"' EXIT HUP INT TERM
archive=check-now-linux-$arch.tar.gz
base=$REPOSITORY/releases
if [ "$release" = latest ]; then
  base=$base/latest/download
else
  base=$base/download/$release
fi
download "$base/$archive" "$temp/$archive"
download "$base/$archive.sha256" "$temp/$archive.sha256"
(
  cd "$temp"
  sha256sum --check --strict "$archive.sha256"
)
mkdir "$temp/unpacked"
tar -xzf "$temp/$archive" -C "$temp/unpacked"
[ -x "$temp/unpacked/check-now" ] || die 'release archive does not contain an executable check-now binary'
[ -d "$temp/unpacked/plugins" ] || die 'release archive does not contain built-in plugins'

# Prepare the complete replacement before moving any existing installed files.
stage=$PREFIX/.check-now-stage.$$
backup=$PREFIX/.check-now-backup.$$
mkdir -p "$PREFIX/bin" "$PREFIX/share"
mkdir "$stage" "$backup"
cp "$temp/unpacked/check-now" "$stage/check-now"
cp -R "$temp/unpacked/plugins" "$stage/plugins"
cp "$temp/unpacked/check-now.sh" "$temp/unpacked/check-now.fish" "$stage/"
cp "$temp/unpacked/VERSION" "$stage/"
if [ -d "$PREFIX/share/check-now/completions" ]; then
  cp -R "$PREFIX/share/check-now/completions" "$stage/completions"
fi
chmod 755 "$stage/check-now"

had_bin=false
had_share=false
new_bin=false
new_share=false
rollback() {
  status=$?
  restore_failed=false
  trap - EXIT HUP INT TERM
  if [ "$status" -ne 0 ]; then
    if [ "$new_bin" = true ]; then rm -f "$PREFIX/bin/check-now"; fi
    if [ "$new_share" = true ]; then rm -rf "$PREFIX/share/check-now"; fi
    if [ "$had_bin" = true ] && ! mv "$backup/check-now" "$PREFIX/bin/check-now"; then restore_failed=true; fi
    if [ "$had_share" = true ] && ! mv "$backup/share-check-now" "$PREFIX/share/check-now"; then restore_failed=true; fi
  fi
  rm -rf "$stage"
  if [ "$restore_failed" = true ]; then
    printf 'check-now installer: rollback incomplete; previous files remain in %s\n' "$backup" >&2
  else
    rm -rf "$backup"
  fi
  exit "$status"
}
trap rollback EXIT
trap 'exit 1' HUP INT TERM
if [ -e "$PREFIX/bin/check-now" ]; then mv "$PREFIX/bin/check-now" "$backup/check-now"; had_bin=true; fi
if [ -e "$PREFIX/share/check-now" ]; then mv "$PREFIX/share/check-now" "$backup/share-check-now"; had_share=true; fi
mv "$stage/check-now" "$PREFIX/bin/check-now"
new_bin=true
mv "$stage" "$PREFIX/share/check-now"
new_share=true
trap - EXIT HUP INT TERM
rm -rf "$backup"
trap 'rm -rf "$stage" "$backup" "$temp"' EXIT HUP INT TERM

if [ "$SHELL_HOOK" = true ]; then
  case "$SHELL_NAME" in
    bash) profile=${BASH_ENV:-"${HOME:-}/.bashrc"} ;;
    zsh) profile=${ZDOTDIR:-"${HOME:-}"}/.zshrc ;;
    fish) profile=${XDG_CONFIG_HOME:-"${HOME:-}/.config"}/fish/config.fish ;;
    ksh|mksh|dash) profile=${HOME:-}/.profile ;;
  esac
  mkdir -p "$(dirname "$profile")"
  marker='# check-now shell integration'
  if ! grep -Fqx "$marker" "$profile" 2>/dev/null; then
    case "$SHELL_NAME" in fish) hook="source '$PREFIX/share/check-now/check-now.fish'" ;; *) hook=". '$PREFIX/share/check-now/check-now.sh'" ;; esac
    { printf '\n%s\n%s\n' "$marker" "$hook"; } >>"$profile"
  fi
fi

if [ "$SHELL_COMPLETION" = true ]; then
  completion_dir=$PREFIX/share/check-now/completions
  mkdir -p "$completion_dir"
  case "$SHELL_NAME" in
    bash) "$PREFIX/bin/check-now" completion bash >"$completion_dir/check-now.bash"; profile=${BASH_ENV:-"${HOME:-}/.bashrc"}; activation=". '$completion_dir/check-now.bash'" ;;
    zsh) "$PREFIX/bin/check-now" completion zsh >"$completion_dir/_check-now"; profile=${ZDOTDIR:-"${HOME:-}"}/.zshrc; activation="fpath=('$completion_dir' \$fpath); autoload -Uz compinit && compinit; autoload -Uz _check-now; compdef _check-now check-now" ;;
    fish) "$PREFIX/bin/check-now" completion fish >"$completion_dir/check-now.fish"; profile=${XDG_CONFIG_HOME:-"${HOME:-}/.config"}/fish/config.fish; activation="source '$completion_dir/check-now.fish'" ;;
  esac
  mkdir -p "$(dirname "$profile")"
  marker='# check-now shell completion'
  if ! grep -Fqx "$marker" "$profile" 2>/dev/null; then printf '\n%s\n%s\n' "$marker" "$activation" >>"$profile"; fi
fi

installed_version=$(cat "$PREFIX/share/check-now/VERSION")
printf 'Installed check-now %s to %s\n' "$installed_version" "$PREFIX/bin/check-now"
printf 'Add %s/bin to PATH if needed.\n' "$PREFIX"
