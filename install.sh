#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
prefix=${CHECK_NOW_PREFIX:-"$HOME/.local"}
shell=auto
do_install=false
shell_hook=false
shell_completion=false

usage() {
  cat <<EOF
Usage: $0 [--build] [--install] [--shell NAME] [--shell-hook] [--shell-completion]

  --build             build dist/check-now and copy plugins (default)
  --install           install the executable and plugins under CHECK_NOW_PREFIX or ~/.local
  --shell NAME        bash, zsh, fish, ksh, dash, or powershell (default: detect)
  --shell-hook        add the update-check hook to the shell profile
  --shell-completion  install and activate completion (requires --install --shell-hook)
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --build) do_install=false ;;
    --install) do_install=true ;;
    --shell) [ "$#" -ge 2 ] || { usage >&2; exit 2; }; shell=$2; shift ;;
    --shell-hook) shell_hook=true ;;
    --shell-completion) shell_completion=true ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
  shift
done

if [ "$shell_completion" = true ]; then
  [ "$do_install" = true ] || { echo '--shell-completion requires --install' >&2; exit 2; }
  [ "$shell_hook" = true ] || { echo '--shell-completion requires --shell-hook' >&2; exit 2; }
fi

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

case "$shell" in
  bash) profile=${BASH_ENV:-"$HOME/.bashrc"} ;;
  zsh) profile=${ZDOTDIR:-"$HOME"}/.zshrc ;;
  ksh|mksh|dash) profile=$HOME/.profile ;;
  fish) profile=$HOME/.config/fish/config.fish ;;
  powershell) profile= ;;
  *) profile= ;;
esac

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

if [ "$shell" = powershell ] && [ "$shell_hook" = true ]; then
  if command -v pwsh >/dev/null 2>&1; then psbin=pwsh
  elif command -v powershell >/dev/null 2>&1; then psbin=powershell
  else
    if [ "$shell_completion" = true ]; then
      echo '--shell-completion requires pwsh or powershell in PATH to resolve $PROFILE' >&2
      exit 2
    fi
    echo 'PowerShell hook is printed in README; profile was not modified.'
    exit 0
  fi
  profile=$($psbin -NoProfile -Command 'Write-Output $PROFILE' 2>/dev/null || true)
  [ -n "$profile" ] || { echo "could not resolve PowerShell \$PROFILE using $psbin" >&2; exit 2; }
fi

if [ "$shell_hook" = true ]; then
  [ "$do_install" = true ] || { echo '--shell-hook requires --install' >&2; exit 2; }
  case "$shell" in
    bash|zsh|ksh|mksh|dash)
      mkdir -p "$(dirname "$profile")"
      marker='# check-now shell integration'
      if ! grep -Fqx "$marker" "$profile" 2>/dev/null; then
        { printf '\n%s\n' "$marker"; printf '%s\n' ". '$prefix/share/check-now/check-now.sh'"; } >> "$profile"
      fi ;;
    fish)
      mkdir -p "$(dirname "$profile")"
      marker='# check-now shell integration'
      if ! grep -Fqx "$marker" "$profile" 2>/dev/null; then
        { printf '\n%s\n' "$marker"; printf '%s\n' "source '$prefix/share/check-now/check-now.fish'"; } >> "$profile"
      fi ;;
    powershell)
      if [ "$shell_completion" = false ]; then
        echo 'PowerShell hook is printed in README; profile was not modified.'
        exit 0
      fi ;;
    *) echo 'Cannot configure an unknown shell.' >&2; exit 2 ;;
  esac
  [ "$shell" = powershell ] || printf 'Shell hook configured in %s\n' "$profile"
else
  printf 'Shell hook not changed. See README.md for the command for %s.\n' "$shell"
fi

if [ "$shell_completion" = true ]; then
  completion_dir="$prefix/share/check-now/completions"
  mkdir -p "$completion_dir" "$(dirname "$profile")"
  case "$shell" in
    powershell)
      "$prefix/bin/check-now" completion powershell > "$completion_dir/check-now.ps1"
      marker='# check-now shell completion'; activation=". '$completion_dir/check-now.ps1'" ;;
    zsh)
      "$prefix/bin/check-now" completion zsh > "$completion_dir/_check-now"
      marker='# check-now shell completion'; activation="fpath=(\"$completion_dir\" \$fpath); autoload -Uz compinit && compinit; autoload -Uz _check-now; compdef _check-now check-now" ;;
    fish)
      "$prefix/bin/check-now" completion fish > "$completion_dir/check-now.fish"
      marker='# check-now shell completion'; activation="source '$completion_dir/check-now.fish'" ;;
    bash)
      "$prefix/bin/check-now" completion bash > "$completion_dir/check-now.bash"
      marker='# check-now shell completion'; activation=". '$completion_dir/check-now.bash'" ;;
  esac
  if ! grep -Fqx "$marker" "$profile" 2>/dev/null; then
    printf '\n%s\n' "$marker" >> "$profile"
  fi
  if ! grep -Fqx "$activation" "$profile" 2>/dev/null; then
    printf '%s\n' "$activation" >> "$profile"
  fi
  printf 'Shell completion configured in %s\n' "$profile"
fi
