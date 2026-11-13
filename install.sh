#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
prefix=${CHECK_NOW_PREFIX:-"$HOME/.local"}
shell=auto
do_install=false
shell_hook=false
shell_completion=false
rc_file=

usage() {
  cat <<EOF
Usage: $0 [--build] [--install] [--shell NAME] [--shell-hook] [--shell-completion] [--rc-file PATH]

  --build             build dist/check-now and copy plugins (default)
  --install           install the executable and plugins under CHECK_NOW_PREFIX or ~/.local
  --shell NAME        bash, zsh, fish, ksh, dash, or powershell (default: detect)
  --shell-hook        add the update-check hook to the shell profile
  --shell-completion  install and activate completion (requires --install --shell-hook)
  --rc-file PATH      select a shell profile directly, including in scripts
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --build) do_install=false ;;
    --install) do_install=true ;;
    --shell) [ "$#" -ge 2 ] || { usage >&2; exit 2; }; shell=$2; shift ;;
    --rc-file) [ "$#" -ge 2 ] && [ -n "$2" ] || { usage >&2; exit 2; }; rc_file=$2; shift ;;
    --shell-hook) shell_hook=true ;;
    --shell-completion) shell_completion=true ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
  shift
done

if [ -n "$rc_file" ] && [ "$shell_hook" != true ] && [ "$shell_completion" != true ]; then
  echo '--rc-file requires --shell-hook or --shell-completion' >&2
  exit 2
fi

if [ "$shell_completion" = true ]; then
  [ "$do_install" = true ] || { echo '--shell-completion requires --install' >&2; exit 2; }
  [ "$shell_hook" = true ] || { echo '--shell-completion requires --shell-hook' >&2; exit 2; }
fi
if [ "$shell_hook" = true ] && [ "$do_install" != true ]; then
  echo '--shell-hook requires --install' >&2
  exit 2
fi

discover_profile() {
  profile_candidates=
  candidate=
  case "$shell" in
    zsh)
      if command -v zsh >/dev/null 2>&1; then
        candidate=$(zsh -c 'printf "CHECK_NOW_PROFILE=%s/.zshrc\n" "${ZDOTDIR:-$HOME}"' 2>/dev/null | sed -n 's/^CHECK_NOW_PROFILE=//p' | tail -n 1 || true)
        [ -n "$candidate" ] && [ -f "$candidate" ] && add_profile_candidate "$candidate"
      elif [ -n "${ZDOTDIR:-}" ] && [ -f "$ZDOTDIR/.zshrc" ]; then
        add_profile_candidate "$ZDOTDIR/.zshrc"
      elif [ -f "$HOME/.zshrc" ]; then
        add_profile_candidate "$HOME/.zshrc"
      fi ;;
    bash)
      if [ -f "$HOME/.bashrc" ]; then
        add_profile_candidate "$HOME/.bashrc"
      else
        for candidate in "$HOME/.bash_profile" "$HOME/.bash_login" "$HOME/.profile"; do
          if [ -f "$candidate" ]; then add_profile_candidate "$candidate"; break; fi
        done
      fi ;;
    fish)
      candidate=${XDG_CONFIG_HOME:-"$HOME/.config"}/fish/config.fish
      [ -f "$candidate" ] && add_profile_candidate "$candidate" ;;
    ksh|mksh|dash)
      if [ -n "${ENV:-}" ] && [ -f "$ENV" ]; then add_profile_candidate "$ENV"
      elif [ -f "$HOME/.profile" ]; then add_profile_candidate "$HOME/.profile"
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
  [ -r /dev/tty ] && [ -w /dev/tty ] || return 1
  printf '%s' "$1" >/dev/tty
  IFS= read -r prompt_value </dev/tty || { prompt_value=; return 1; }
  return 0
}

resolve_profile() {
  profile=$rc_file
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
    0) printf 'Could not discover an existing profile for %s.\n' "$shell" ;;
    *) printf 'Found multiple possible %s profile files:\n%s\n' "$shell" "$profile_candidates" ;;
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
    '~') profile=$HOME ;;
    '~/'*) profile=$HOME/${profile#\~/} ;;
  esac
  case "$profile" in *'
'*) return 1 ;; esac
  case "$profile" in /*) ;; *) profile=$PWD/$profile ;; esac
  { [ ! -e "$profile" ] && [ ! -L "$profile" ]; } || [ -f "$profile" ]
}

quote_shell_path() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

quote_powershell_path() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/''/g")"
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

if [ "$shell" = auto ]; then
  shell=${SHELL:-}
  if [ -z "$shell" ]; then shell=$(ps -p $$ -o comm= 2>/dev/null || true); fi
  shell=$(basename "$shell")
  case "$shell" in bash|zsh|fish|ksh|mksh|dash|powershell|pwsh) ;; *) shell=unknown ;; esac
else
  shell=$(basename "$shell")
fi
if [ "$shell" = pwsh ]; then shell=powershell; fi
printf 'Detected shell: %s\n' "$shell"

if [ "$shell_hook" = true ] || [ "$shell_completion" = true ]; then
  case "$shell" in
    bash|zsh|fish|ksh|mksh|dash|powershell) ;;
    *) echo "Cannot configure shell '$shell'." >&2; exit 2 ;;
  esac
fi

if [ "$shell_completion" = true ]; then
  case "$shell" in
    bash|zsh|fish|powershell) ;;
    *) echo "--shell-completion does not support shell '$shell'; use bash, zsh, fish, or powershell" >&2; exit 2 ;;
  esac
fi

mkdir -p "$root/dist"
(cd "$root" && go build -o dist/check-now ./cmd/check-now)
rm -rf "$root/dist/plugins"
cp -R "$root/plugins" "$root/dist/plugins"
printf 'Built: %s\n' "$root/dist/check-now"

if [ "$do_install" = true ]; then
  mkdir -p "$prefix/bin" "$prefix/share/check-now"
  cp "$root/dist/check-now" "$prefix/bin/check-now"
  rm -rf "$prefix/share/check-now/plugins"
  cp -R "$root/dist/plugins" "$prefix/share/check-now/plugins"
  cp "$root/check-now.sh" "$root/check-now.fish" "$prefix/share/check-now/"
  printf 'Installed: %s/bin/check-now\n' "$prefix"
  printf 'Plugins: %s/share/check-now/plugins\n' "$prefix"
fi

if [ "$shell_hook" = true ] || [ "$shell_completion" = true ]; then
  if resolve_profile; then
    case "$shell" in
      fish) hook_file=$prefix/share/check-now/check-now.fish; hook="source $(quote_shell_path "$hook_file")" ;;
      powershell) hook_file=; hook='check-now shell | Out-Host' ;;
      *) hook_file=$prefix/share/check-now/check-now.sh; hook=". $(quote_shell_path "$hook_file")" ;;
    esac
    if [ "$shell_hook" = true ] && [ -n "$hook" ]; then
      ensure_profile_entry '# check-now shell integration' "$hook"
      if [ -n "$hook_file" ]; then printf 'Shell hook script: %s\n' "$hook_file"
      else printf 'Shell hook command: %s\n' "$hook"; fi
      if [ "$entry_state" = added ]; then printf 'Shell hook added to %s\n' "$profile"
      else printf 'Shell hook already present in %s\n' "$profile"; fi
    fi

    if [ "$shell_completion" = true ]; then
      completion_dir="$prefix/share/check-now/completions"
      mkdir -p "$completion_dir"
      case "$shell" in
        powershell) completion_file=$completion_dir/check-now.ps1; activation=". $(quote_powershell_path "$completion_file")" ;;
        zsh) completion_file=$completion_dir/_check-now; activation="fpath=($(quote_shell_path "$completion_dir") \$fpath); autoload -Uz compinit && compinit; autoload -Uz _check-now; compdef _check-now check-now" ;;
        fish) completion_file=$completion_dir/check-now.fish; activation="source $(quote_shell_path "$completion_file")" ;;
        bash) completion_file=$completion_dir/check-now.bash; activation=". $(quote_shell_path "$completion_file")" ;;
      esac
      "$prefix/bin/check-now" completion "$shell" >"$completion_file"
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
