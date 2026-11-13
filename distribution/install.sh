#!/bin/sh
set -eu

REPOSITORY=https://github.com/teelabs/check-now
PREFIX=${CHECK_NOW_PREFIX:-"${HOME:-}/.local"}
VERSION=latest
SHELL_HOOK=false
SHELL_COMPLETION=false
SHELL_NAME=auto
RC_FILE=

usage() {
  cat <<'EOF'
Usage: install.sh [--version latest|X.Y.Z] [--prefix DIR]
                  [--shell NAME] [--shell-hook] [--shell-completion]
                  [--rc-file PATH]

Downloads a prebuilt Linux release from GitHub and verifies its SHA-256 checksum.
Supported targets are Linux amd64 and arm64. The default prefix is ~/.local.
Shell integration is optional and never runs an update command.
The installer discovers a profile candidate and asks before changing it.
Use --rc-file PATH to select a profile directly, including in scripts.
EOF
}

die() { printf 'check-now installer: %s\n' "$*" >&2; exit 1; }

discover_profile() {
  profile_candidates=
  candidate=
  case "$SHELL_NAME" in
    zsh)
      if command -v zsh >/dev/null 2>&1; then
        candidate=$(zsh -c 'printf "CHECK_NOW_PROFILE=%s/.zshrc\n" "${ZDOTDIR:-$HOME}"' 2>/dev/null | sed -n 's/^CHECK_NOW_PROFILE=//p' | tail -n 1 || true)
        [ -n "$candidate" ] && [ -f "$candidate" ] && add_profile_candidate "$candidate"
      elif [ -n "${ZDOTDIR:-}" ] && [ -f "$ZDOTDIR/.zshrc" ]; then
        add_profile_candidate "$ZDOTDIR/.zshrc"
      elif [ -f "${HOME:-}/.zshrc" ]; then
        add_profile_candidate "$HOME/.zshrc"
      fi ;;
    bash)
      if [ -f "${HOME:-}/.bashrc" ]; then
        add_profile_candidate "$HOME/.bashrc"
      else
        for candidate in "${HOME:-}/.bash_profile" "${HOME:-}/.bash_login" "${HOME:-}/.profile"; do
          if [ -f "$candidate" ]; then add_profile_candidate "$candidate"; break; fi
        done
      fi ;;
    fish)
      candidate=${XDG_CONFIG_HOME:-"${HOME:-}/.config"}/fish/config.fish
      [ -f "$candidate" ] && add_profile_candidate "$candidate" ;;
    ksh|mksh|dash|sh)
      if [ -n "${ENV:-}" ] && [ -f "$ENV" ]; then add_profile_candidate "$ENV"
      elif [ -f "${HOME:-}/.profile" ]; then add_profile_candidate "$HOME/.profile"
      fi ;;
    powershell)
      if command -v pwsh >/dev/null 2>&1; then psbin=pwsh
      elif command -v powershell >/dev/null 2>&1; then psbin=powershell
      else psbin=; fi
      if [ -n "$psbin" ]; then
        candidate=$($psbin -NoProfile -Command 'Write-Output "CHECK_NOW_PROFILE=$PROFILE"' 2>/dev/null | sed -n 's/^CHECK_NOW_PROFILE=//p' | tail -n 1 || true)
        [ -n "$candidate" ] && [ -f "$candidate" ] && add_profile_candidate "$candidate"
      fi ;;
  esac
}

add_profile_candidate() {
  case "$1" in *'
'*) return ;; esac
  if printf '%s\n' "$profile_candidates" | grep -Fqx "$1"; then return; fi
  if [ -n "$profile_candidates" ]; then profile_candidates="$profile_candidates
$1"
  else profile_candidates=$1
  fi
}

prompt_value() {
  prompt_text=$1
  [ -r /dev/tty ] && [ -w /dev/tty ] || return 1
  printf '%s' "$prompt_text" >/dev/tty
  IFS= read -r prompt_value </dev/tty || { prompt_value=; return 1; }
  return 0
}

resolve_profile() {
  profile=$RC_FILE
  if [ -n "$profile" ]; then
    if ! normalize_profile_path; then
      printf 'Profile path must name a regular file or a path that does not exist yet; multiline values are not allowed.\n'
      profile=
      return 1
    fi
    return 0
  fi

  discover_profile
  candidate_count=$(printf '%s\n' "$profile_candidates" | awk 'NF { count++ } END { print count+0 }')
  case "$candidate_count" in
    1)
      candidate=$profile_candidates
      printf 'Discovered shell profile: %s\n' "$candidate"
      if ! prompt_value 'Use this profile? [Y/n] '; then profile=; return 1; fi
      case "$prompt_value" in
        ''|y|Y|yes|YES|Yes) profile=$candidate; return 0 ;;
        n|N|no|NO|No) ;;
        *) printf 'Profile not confirmed.\n' ;;
      esac
      ;;
    0)
      printf 'Could not discover an existing profile for %s.\n' "$SHELL_NAME"
      ;;
    *)
      printf 'Found multiple possible %s profile files:\n%s\n' "$SHELL_NAME" "$profile_candidates"
      ;;
  esac

  if ! prompt_value 'Enter the shell profile path (leave blank to skip integration): '; then profile=; return 1; fi
  profile=$prompt_value
  [ -n "$profile" ] || return 1
  if ! normalize_profile_path; then
    printf 'Profile path must name a regular file or a path that does not exist yet; multiline values are not allowed.\n'
    profile=
    return 1
  fi
  return 0
}

normalize_profile_path() {
  case "$profile" in
    '~') profile=${HOME:-} ;;
    '~/'*) profile=${HOME:-}/${profile#\~/} ;;
  esac
  case "$profile" in *'
'*) return 1 ;; esac
  case "$profile" in /*) ;; *) profile=$PWD/$profile ;; esac
  { [ ! -e "$profile" ] && [ ! -L "$profile" ]; } || [ -f "$profile" ]
}

quote_shell_path() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

ensure_profile_entry() {
  entry_marker=$1
  entry_line=$2
  entry_state=already
  mkdir -p "$(dirname "$profile")"
  touch "$profile"
  if ! grep -Fqx "$entry_marker" "$profile" 2>/dev/null; then
    append_profile_line "$entry_marker"
    entry_state=added
  fi
  if ! grep -Fqx "$entry_line" "$profile" 2>/dev/null; then
    append_profile_line "$entry_line"
    entry_state=added
  fi
}

append_profile_line() {
  if [ -s "$profile" ] && [ -n "$(tail -c 1 "$profile")" ]; then
    printf '\n' >>"$profile"
  fi
  printf '%s\n' "$1" >>"$profile"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) [ "$#" -ge 2 ] || die '--version requires latest or X.Y.Z'; VERSION=$2; shift 2 ;;
    --prefix) [ "$#" -ge 2 ] || die '--prefix requires a directory'; PREFIX=$2; shift 2 ;;
    --shell) [ "$#" -ge 2 ] || die '--shell requires a name'; SHELL_NAME=$2; shift 2 ;;
    --rc-file) [ "$#" -ge 2 ] && [ -n "$2" ] || die '--rc-file requires a path'; RC_FILE=$2; shift 2 ;;
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
if [ -n "$RC_FILE" ] && [ "$SHELL_HOOK" != true ] && [ "$SHELL_COMPLETION" != true ]; then
  die '--rc-file requires --shell-hook or --shell-completion'
fi

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

if [ "$SHELL_HOOK" = true ] || [ "$SHELL_COMPLETION" = true ]; then
  if resolve_profile; then
    case "$SHELL_NAME" in
      fish) hook_file=$PREFIX/share/check-now/check-now.fish; hook="source $(quote_shell_path "$hook_file")" ;;
      powershell) hook_file=; hook= ;;
      *) hook_file=$PREFIX/share/check-now/check-now.sh; hook=". $(quote_shell_path "$hook_file")" ;;
    esac
    if [ "$SHELL_HOOK" = true ] && [ -n "$hook" ]; then
      ensure_profile_entry '# check-now shell integration' "$hook"
      printf 'Shell hook script: %s\n' "$hook_file"
      if [ "$entry_state" = added ]; then printf 'Shell hook added to %s\n' "$profile"
      else printf 'Shell hook already present in %s\n' "$profile"; fi
    fi

    if [ "$SHELL_COMPLETION" = true ]; then
      completion_dir=$PREFIX/share/check-now/completions
      mkdir -p "$completion_dir"
      case "$SHELL_NAME" in
        bash) completion_file=$completion_dir/check-now.bash; activation=". $(quote_shell_path "$completion_file")" ;;
        zsh) completion_file=$completion_dir/_check-now; activation="fpath=($(quote_shell_path "$completion_dir") \$fpath); autoload -Uz compinit && compinit; autoload -Uz _check-now; compdef _check-now check-now" ;;
        fish) completion_file=$completion_dir/check-now.fish; activation="source $(quote_shell_path "$completion_file")" ;;
        *) completion_file=$completion_dir/check-now.ps1; activation=". $(quote_shell_path "$completion_file")" ;;
      esac
      "$PREFIX/bin/check-now" completion "$SHELL_NAME" >"$completion_file"
      printf 'Completion script installed at %s\n' "$completion_file"
      ensure_profile_entry '# check-now shell completion' "$activation"
      if [ "$entry_state" = added ]; then printf 'Shell completion activation added to %s\n' "$profile"
      else printf 'Shell completion activation already present in %s\n' "$profile"; fi
    fi
    printf 'Start a new shell or reload %s to use the changes.\n' "$profile"
  else
    printf 'No shell profile was confirmed; shell hook and completions were not installed.\n'
    printf 'Run the installer again with --rc-file PATH and the same shell setup options for full configuration.\n'
  fi
else
  printf 'Shell hook and completions were not requested; shell profile files were not changed.\n'
fi

installed_version=$(cat "$PREFIX/share/check-now/VERSION")
printf 'Installed check-now %s to %s\n' "$installed_version" "$PREFIX/bin/check-now"
printf 'Add %s/bin to PATH if needed.\n' "$PREFIX"
